//go:build windows && cgo

// Package winmedia 將 Media Foundation／D3D11 限定在各自固定的 COM 執行緒。
package winmedia

/*
#cgo CXXFLAGS: -std=c++17 -D_WIN32_WINNT=0x0A00 -DWINVER=0x0A00
#cgo LDFLAGS: -static -static-libstdc++ -static-libgcc -lmfplat -lmf -lmfuuid -lmfreadwrite -ld3d11 -ldxgi -lole32 -loleaut32 -luuid -lstrmiids
#include "native.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
	"yourdesk/internal/ownedworker"
)

type result struct {
	data, config []byte
	image        *image.RGBA
	backend      string
	software     bool
	err          error
}
type Session struct {
	worker *ownedworker.Worker[*C.yd_media, result]
}

// 驅動卡住後不在同一程序反覆建立工作者；既有串流退回軟體處理。
var nativeStalled atomic.Bool

const operationTimeout = 5 * time.Second
const closeTimeout = time.Second

func New(mode int) (*Session, error) {
	if nativeStalled.Load() {
		return nil, fmt.Errorf("Windows 原生影音工作者已停滯，本次啟動使用軟體備援")
	}
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	w, err := ownedworker.New[*C.yd_media, result](ctx, func() (*C.yd_media, error) {
		var native *C.yd_media
		if hr := C.yd_media_open(C.int(mode), &native); hr < 0 {
			return nil, failure("建立硬體裝置", hr)
		}
		return native, nil
	}, func(native *C.yd_media) { C.yd_media_close(native) })
	if err != nil {
		if errors.Is(err, ownedworker.ErrStalled) {
			nativeStalled.Store(true)
		}
		return nil, err
	}
	return &Session{worker: w}, nil
}
func failure(action string, hr C.int) error {
	return fmt.Errorf("Windows %s失敗（HRESULT 0x%08X）：%s", action, uint32(hr), C.GoString(C.yd_media_error()))
}
func (s *Session) call(run func(*C.yd_media) result) result {
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	r, err := s.worker.Call(ctx, run)
	if err != nil {
		if errors.Is(err, ownedworker.ErrStalled) {
			nativeStalled.Store(true)
		}
		return result{err: err}
	}
	return r
}
func (s *Session) Close() {
	if errors.Is(s.worker.Err(), ownedworker.ErrStalled) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	if err := s.worker.Close(ctx); err != nil {
		nativeStalled.Store(true)
	}
}
func (s *Session) Encode(src *image.RGBA, bitrate, fps, quality, gop int) ([]byte, []byte, string, error) {
	return s.EncodeCodec(src, bitrate, fps, quality, gop, 1)
}
func (s *Session) EncodeCodec(src *image.RGBA, bitrate, fps, quality, gop, codec int) ([]byte, []byte, string, error) {
	if !validRGBA(src) {
		return nil, nil, "", fmt.Errorf("無效的 RGBA 影格")
	}
	src = ownedRGBA(src)
	r := s.call(func(native *C.yd_media) result {
		defer releaseOwnedRGBA(src)
		var out C.yd_media_output
		defer C.yd_media_free(&out)
		hr := C.yd_media_encode_format(native, C.int(codec), (*C.uchar)(unsafe.Pointer(&src.Pix[0])), C.int(src.Bounds().Dx()), C.int(src.Bounds().Dy()), C.int(src.Stride), C.int(bitrate), C.int(fps), C.int(quality), C.int(gop), &out)
		if hr < 0 {
			return result{err: failure("影片硬體編碼", hr)}
		}
		return result{data: C.GoBytes(unsafe.Pointer(out.data), C.int(out.size)), config: C.GoBytes(unsafe.Pointer(out.config), C.int(out.config_size)), backend: C.GoString(C.yd_media_backend(native))}
	})
	runtime.KeepAlive(src)
	return r.data, r.config, r.backend, r.err
}
func (s *Session) Decode(data []byte, width, height int) (*image.RGBA, string, error) {
	image, backend, _, err := s.DecodeCodec(data, width, height, 1)
	return image, backend, err
}
func (s *Session) DecodeCodec(data []byte, width, height, codec int) (*image.RGBA, string, bool, error) {
	if len(data) == 0 || len(data) > 32<<20 {
		return nil, "", false, fmt.Errorf("無效的影片影格")
	}
	data = append([]byte(nil), data...)
	r := s.call(func(native *C.yd_media) result {
		var out C.yd_media_output
		defer C.yd_media_free(&out)
		hr := C.yd_media_decode_format(native, C.int(codec), (*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)), C.int(width), C.int(height), &out)
		if hr < 0 {
			return result{err: failure("影片解碼", hr)}
		}
		w, h := int(out.width), int(out.height)
		if w < 1 || h < 1 || w > 8192 || h > 8192 || w*h > 32<<20 || uint64(out.size) != uint64(w*h*4) {
			return result{err: fmt.Errorf("硬體解碼輸出尺寸無效")}
		}
		return result{image: &image.RGBA{Pix: C.GoBytes(unsafe.Pointer(out.data), C.int(out.size)), Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, backend: C.GoString(C.yd_media_backend(native)), software: C.yd_media_decoder_software(native) != 0}
	})
	return r.image, r.backend, r.software, r.err
}
func (s *Session) Scale(src, dst *image.RGBA) (string, error) {
	if !validRGBA(src) || !validRGBA(dst) {
		return "", fmt.Errorf("無效的縮圖緩衝")
	}
	src = ownedRGBA(src)
	output := image.NewRGBA(image.Rect(0, 0, dst.Bounds().Dx(), dst.Bounds().Dy()))
	r := s.call(func(native *C.yd_media) result {
		defer releaseOwnedRGBA(src)
		hr := C.yd_media_scale(native, (*C.uchar)(unsafe.Pointer(&src.Pix[0])), C.int(src.Bounds().Dx()), C.int(src.Bounds().Dy()), C.int(src.Stride), (*C.uchar)(unsafe.Pointer(&output.Pix[0])), C.int(output.Bounds().Dx()), C.int(output.Bounds().Dy()), C.int(output.Stride))
		if hr < 0 {
			return result{err: failure("D3D11 縮圖", hr)}
		}
		return result{backend: C.GoString(C.yd_media_backend(native)), image: output}
	})
	runtime.KeepAlive(src)
	if r.err == nil {
		for y := 0; y < output.Bounds().Dy(); y++ {
			copy(dst.Pix[y*dst.Stride:y*dst.Stride+output.Stride], output.Pix[y*output.Stride:(y+1)*output.Stride])
		}
	}
	return r.backend, r.err
}
