package video

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"yourdesk/internal/optimization"
	"yourdesk/internal/ownedworker"
)

const nativeCodecTimeout = 5 * time.Second

type codecOutput struct {
	image         image.Image
	data          []byte
	backend, mode string
	err           error
}
type codecReport struct{ backend, mode string }
type codecResource interface{ Close() error }

// 輸入先移交獨立副本，資源僅由原生工作者操作與回收；逾時後不強制
// 銷毀仍在驅動內的 session，也不在同一後端反覆建立停滯執行緒。
type boundedCodec[T codecResource] struct {
	worker  *ownedworker.Worker[T, codecOutput]
	stalled *atomic.Bool
	report  atomic.Pointer[codecReport]
}

func newBoundedCodec[T codecResource](stalled *atomic.Bool, create func() (T, error)) (*boundedCodec[T], error) {
	if stalled.Load() {
		return nil, fmt.Errorf("原生影音後端已因逾時停用，本次程序使用其他編解碼備援")
	}
	ctx, cancel := context.WithTimeout(context.Background(), nativeCodecTimeout)
	defer cancel()
	w, err := ownedworker.New[T, codecOutput](ctx, create, func(resource T) { _ = resource.Close() })
	if err != nil {
		if errors.Is(err, ownedworker.ErrStalled) {
			stalled.Store(true)
		}
		return nil, err
	}
	return &boundedCodec[T]{worker: w, stalled: stalled}, nil
}
func (c *boundedCodec[T]) call(run func(T) codecOutput) codecOutput {
	ctx, cancel := context.WithTimeout(context.Background(), nativeCodecTimeout)
	defer cancel()
	r, err := c.worker.Call(ctx, run)
	if err != nil {
		if errors.Is(err, ownedworker.ErrStalled) {
			c.stalled.Store(true)
		}
		return codecOutput{err: err}
	}
	if r.backend != "" || r.mode != "" {
		c.report.Store(&codecReport{r.backend, r.mode})
	}
	return r
}
func (c *boundedCodec[T]) Close() error {
	if errors.Is(c.worker.Err(), ownedworker.ErrStalled) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := c.worker.Close(ctx)
	if err != nil {
		c.stalled.Store(true)
	}
	return err
}
func (c *boundedCodec[T]) Backend() string {
	if r := c.report.Load(); r != nil {
		return r.backend
	}
	return "unknown"
}
func (c *boundedCodec[T]) DecodingMode() string {
	if r := c.report.Load(); r != nil {
		return r.mode
	}
	return "unknown"
}

type boundedDecoder struct{ *boundedCodec[IntraDecoder] }

func boundDecoder(stalled *atomic.Bool, create func() (IntraDecoder, error)) (IntraDecoder, error) {
	c, err := newBoundedCodec(stalled, create)
	if err != nil {
		return nil, err
	}
	return &boundedDecoder{c}, nil
}
func (d *boundedDecoder) Decode(payload []byte) (image.Image, error) {
	if len(payload) == 0 || len(payload) > 32<<20 {
		return nil, ErrVideoUnavailable
	}
	payload = slices.Clone(payload)
	r := d.call(func(resource IntraDecoder) codecOutput {
		img, err := resource.Decode(payload)
		backend, mode := "unknown", "unknown"
		if value, ok := resource.(BackendReporter); ok {
			backend = value.Backend()
		}
		if value, ok := resource.(interface{ DecodingMode() string }); ok {
			mode = value.DecodingMode()
		}
		return codecOutput{image: img, backend: backend, mode: mode, err: err}
	})
	return r.image, r.err
}
func (d *boundedDecoder) SetDecodePolicy(p optimization.Policy) {
	p.Decoders = slices.Clone(p.Decoders)
	d.call(func(resource IntraDecoder) codecOutput {
		if value, ok := resource.(interface{ SetDecodePolicy(optimization.Policy) }); ok {
			value.SetDecodePolicy(p)
		}
		return codecOutput{}
	})
}

type boundedEncoder struct {
	*boundedCodec[IntraEncoder]
	pixels sync.Pool
}

func boundEncoder(stalled *atomic.Bool, create func() (IntraEncoder, error)) (IntraEncoder, error) {
	c, err := newBoundedCodec(stalled, create)
	if err != nil {
		return nil, err
	}
	return &boundedEncoder{boundedCodec: c}, nil
}
func (e *boundedEncoder) Encode(src image.Image, quality int) ([]byte, error) {
	if src == nil {
		return nil, ErrVideoUnavailable
	}
	b := src.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 || b.Dx() > 8192 || b.Dy() > 8192 || int64(b.Dx())*int64(b.Dy()) > 32<<20 {
		return nil, ErrVideoUnavailable
	}
	copy, _ := e.pixels.Get().(*image.RGBA)
	if copy == nil || copy.Bounds().Size() != b.Size() {
		copy = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	}
	draw.Draw(copy, copy.Bounds(), src, b.Min, draw.Src)
	r := e.call(func(resource IntraEncoder) codecOutput {
		defer e.pixels.Put(copy)
		data, err := resource.Encode(copy, quality)
		backend := "unknown"
		if value, ok := resource.(BackendReporter); ok {
			backend = value.Backend()
		}
		return codecOutput{data: data, backend: backend, err: err}
	})
	return r.data, r.err
}
func (e *boundedEncoder) SetRate(rate, fps int) {
	e.call(func(resource IntraEncoder) codecOutput {
		if value, ok := resource.(RateEncoder); ok {
			value.SetRate(rate, fps)
		}
		return codecOutput{}
	})
}
func (e *boundedEncoder) SetKeyframeInterval(gop int) {
	e.call(func(resource IntraEncoder) codecOutput {
		if value, ok := resource.(GOPEncoder); ok {
			value.SetKeyframeInterval(gop)
		}
		return codecOutput{}
	})
}
