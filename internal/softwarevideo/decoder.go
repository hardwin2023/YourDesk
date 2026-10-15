//go:build cgo && ffmpeg

// Package softwarevideo 提供隨程式發行、不啟動外部程序的 FFmpeg 編解碼器。
package softwarevideo

/*
#cgo !windows LDFLAGS: -lavcodec -lswscale -lavutil
// 執行期路徑僅由此套件提供，避免全域 CGO_LDFLAGS 在各套件重複累加。
#cgo darwin LDFLAGS: -Wl,-rpath,@loader_path -Wl,-rpath,@loader_path/../Frameworks
#cgo windows LDFLAGS: -l:libavcodec.dll.a -l:libswscale.dll.a -l:libavutil.dll.a
#include <libavcodec/avcodec.h>
#include <libavutil/imgutils.h>
#include <libavutil/hwcontext.h>
#include <libswscale/swscale.h>
#include <stdlib.h>
#include <string.h>
typedef struct { AVCodecContext *codec; AVFrame *frame; struct SwsContext *scale; } yd_sw_decoder;
static void yd_sw_close(yd_sw_decoder *d) {
 if (!d) return;
 avcodec_free_context(&d->codec); av_frame_free(&d->frame); sws_freeContext(d->scale); free(d);
}
static enum AVPixelFormat yd_av1_hw_format(AVCodecContext *ctx,const enum AVPixelFormat *formats) {
 for (const enum AVPixelFormat *p=formats;*p!=AV_PIX_FMT_NONE;p++) if(*p==AV_PIX_FMT_VIDEOTOOLBOX)return *p;
 return AV_PIX_FMT_NONE;
}
static yd_sw_decoder *yd_sw_open(int kind) {
 // 按名稱指定純 CPU 實作，不能由系統硬體 decoder 代替。
 const char *name = kind==1 ? "h264" : kind==2 ? "hevc" : kind==3 ? "libaom-av1" : kind==4 ? "av1" : NULL;
 const AVCodec *codec = name ? avcodec_find_decoder_by_name(name) : NULL;
 if (!codec) return NULL;
 yd_sw_decoder *d = calloc(1, sizeof(*d));
 if (!d) return NULL;
 d->codec = avcodec_alloc_context3(codec); d->frame = av_frame_alloc();
 if (!d->codec || !d->frame) { yd_sw_close(d); return NULL; }
 d->codec->thread_count = 4;
 // slice threading 保留 GOP 狀態但不累積 frame threading 延遲。
 d->codec->thread_type = FF_THREAD_SLICE;
 d->codec->flags |= AV_CODEC_FLAG_LOW_DELAY;
 d->codec->max_pixels = 32LL << 20;
 d->codec->err_recognition = AV_EF_CAREFUL | AV_EF_EXPLODE;
 if (kind==4) {
  d->codec->get_format=yd_av1_hw_format;
  if(av_hwdevice_ctx_create(&d->codec->hw_device_ctx,AV_HWDEVICE_TYPE_VIDEOTOOLBOX,NULL,NULL,0)<0){yd_sw_close(d);return NULL;}
 }
 if (avcodec_open2(d->codec, codec, NULL) < 0) { yd_sw_close(d); return NULL; }
 return d;
}
static int yd_sw_decode(yd_sw_decoder *d, const unsigned char *data, int size, unsigned char **rgba, int *w, int *h) {
 if (!d || size<=0 || size>32*1024*1024) return AVERROR(EINVAL);
 AVPacket *packet=av_packet_alloc();
 if (!packet) return AVERROR(ENOMEM);
 int status=av_new_packet(packet,size);
 if (status>=0) {
  memcpy(packet->data,data,size); // av_new_packet 提供 codec 所需的尾端零填充。
  status=avcodec_send_packet(d->codec,packet);
 }
 av_packet_free(&packet);
 if (status<0) return status;
 av_frame_unref(d->frame);
 status=avcodec_receive_frame(d->codec,d->frame);
 if (status<0) return status;
 if(d->frame->format==AV_PIX_FMT_VIDEOTOOLBOX) {
  AVFrame *cpu=av_frame_alloc();if(!cpu)return AVERROR(ENOMEM);
  status=av_hwframe_transfer_data(cpu,d->frame,0);
  if(status>=0)status=av_frame_copy_props(cpu,d->frame);
  if(status<0){av_frame_free(&cpu);return status;}
  av_frame_unref(d->frame);av_frame_move_ref(d->frame,cpu);av_frame_free(&cpu);
 }
 AVFrame *f=d->frame;
 if (f->width<=0 || f->height<=0 || f->width>8192 || f->height>8192 || (int64_t)f->width*f->height>(32LL<<20)) return AVERROR(EINVAL);
 d->scale=sws_getCachedContext(d->scale,f->width,f->height,f->format,f->width,f->height,AV_PIX_FMT_RGBA,SWS_BILINEAR,NULL,NULL,NULL);
 if (!d->scale) return AVERROR(ENOMEM);
 int space=f->colorspace==AVCOL_SPC_BT709 ? SWS_CS_ITU709 : f->colorspace==AVCOL_SPC_BT2020_NCL ? SWS_CS_BT2020 : SWS_CS_ITU601;
 const int *coeff=sws_getCoefficients(space);
 status=sws_setColorspaceDetails(d->scale,coeff,f->color_range==AVCOL_RANGE_JPEG,coeff,1,0,1<<16,1<<16);
 if (status<0) return status;
 unsigned char *out=av_malloc((size_t)f->width*f->height*4);
 if (!out) return AVERROR(ENOMEM);
 unsigned char *planes[4]={out,NULL,NULL,NULL};int strides[4]={f->width*4,0,0,0};
 status=sws_scale(d->scale,(const unsigned char *const *)f->data,f->linesize,0,f->height,planes,strides);
 if (status!=f->height) { av_free(out); return AVERROR(EINVAL); }
 *rgba=out;*w=f->width;*h=f->height;return 0;
}
*/
import "C"
import (
	"context"
	"errors"
	"fmt"
	"image"
	"sync/atomic"
	"time"
	"unsafe"
	"yourdesk/internal/ownedworker"
)

type Decoder struct {
	worker *ownedworker.Worker[*C.yd_sw_decoder, decodeResult]
	codec  int
}

type decodeResult struct {
	image *image.RGBA
	err   error
}

// 相同後端逾時後，本次程序不再建立可能停滯的原生工作者。
var stalledDecoders atomic.Uint32

const decodeTimeout = 5 * time.Second

func New(codec int) (*Decoder, error) {
	if codec < 1 || codec > 4 || stalledDecoders.Load()&(1<<codec) != 0 {
		return nil, fmt.Errorf("FFmpeg 解碼器不可用或已因逾時停用：%d", codec)
	}
	ctx, cancel := context.WithTimeout(context.Background(), decodeTimeout)
	defer cancel()
	w, err := ownedworker.New[*C.yd_sw_decoder, decodeResult](ctx, func() (*C.yd_sw_decoder, error) {
		d := C.yd_sw_open(C.int(codec))
		if d == nil {
			return nil, fmt.Errorf("FFmpeg CPU 解碼器不可用：%d", codec)
		}
		return d, nil
	}, func(d *C.yd_sw_decoder) { C.yd_sw_close(d) })
	if err != nil {
		if errors.Is(err, ownedworker.ErrStalled) {
			stalledDecoders.Or(1 << codec)
		}
		return nil, err
	}
	return &Decoder{worker: w, codec: codec}, nil
}
func (d *Decoder) Decode(data []byte) (*image.RGBA, error) {
	if len(data) == 0 || len(data) > 32<<20 {
		return nil, fmt.Errorf("無效軟解工作階段或影格")
	}
	// 逾時返回時，原生呼叫仍可能尚未讀完輸入。
	data = append([]byte(nil), data...)
	ctx, cancel := context.WithTimeout(context.Background(), decodeTimeout)
	defer cancel()
	r, err := d.worker.Call(ctx, func(native *C.yd_sw_decoder) decodeResult {
		var out *C.uchar
		var w, h C.int
		status := C.yd_sw_decode(native, (*C.uchar)(unsafe.Pointer(&data[0])), C.int(len(data)), &out, &w, &h)
		if out != nil {
			defer C.av_free(unsafe.Pointer(out))
		}
		if status < 0 {
			var msg [256]C.char
			C.av_strerror(status, &msg[0], 256)
			return decodeResult{err: fmt.Errorf("FFmpeg 軟解失敗：%s", C.GoString(&msg[0]))}
		}
		return decodeResult{image: &image.RGBA{Pix: C.GoBytes(unsafe.Pointer(out), w*h*4), Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}}
	})
	if err != nil {
		if errors.Is(err, ownedworker.ErrStalled) {
			stalledDecoders.Or(1 << d.codec)
		}
		return nil, err
	}
	return r.image, r.err
}
func (d *Decoder) Close() error {
	if errors.Is(d.worker.Err(), ownedworker.ErrStalled) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := d.worker.Close(ctx)
	if err != nil {
		stalledDecoders.Or(1 << d.codec)
	}
	return err
}
func (d *Decoder) Backend() string {
	return map[int]string{1: "FFmpeg H.264 CPU", 2: "FFmpeg HEVC CPU", 3: "FFmpeg / libaom AV1 CPU", 4: "VideoToolbox AV1 / FFmpeg"}[d.codec]
}
func (d *Decoder) DecodingMode() string {
	if d.codec == 4 {
		return "hardware"
	}
	return "software"
}
