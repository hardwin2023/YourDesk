package video

import (
	"context"
	"errors"
	"image"
	"image/color"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"yourdesk/internal/ownedworker"
)

type blockedEncoder struct {
	entered, release, closed chan struct{}
	pixel                    chan color.Color
}

func (e *blockedEncoder) Encode(img image.Image, quality int) ([]byte, error) {
	close(e.entered)
	<-e.release
	e.pixel <- img.At(0, 0)
	return []byte{1}, nil
}
func (e *blockedEncoder) Close() error { close(e.closed); return nil }

type blockedDecoder struct {
	entered, release, closed chan struct{}
	data                     chan byte
}

func (d *blockedDecoder) Decode(data []byte) (image.Image, error) {
	close(d.entered)
	<-d.release
	d.data <- data[0]
	return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
}
func (d *blockedDecoder) Close() error         { close(d.closed); return nil }
func (d *blockedDecoder) Backend() string      { return "測試後端" }
func (d *blockedDecoder) DecodingMode() string { return "hardware" }

func TestBoundedEncoderRetainsOwnedPixelsAfterTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		raw := &blockedEncoder{make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan color.Color, 1)}
		var stalled atomic.Bool
		e, err := boundEncoder(&stalled, func() (IntraEncoder, error) { return raw, nil })
		if err != nil {
			t.Fatal(err)
		}
		src := image.NewRGBA(image.Rect(0, 0, 2, 2))
		src.SetRGBA(0, 0, color.RGBA{10, 20, 30, 255})
		done := make(chan error, 1)
		go func() { _, err := e.Encode(src, 80); done <- err }()
		<-raw.entered
		if err := <-done; !errors.Is(err, ownedworker.ErrStalled) {
			t.Fatal(err)
		}
		src.SetRGBA(0, 0, color.RGBA{})
		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-raw.closed:
			t.Fatal("原生編碼尚未返回就釋放資源")
		default:
		}
		if _, err := boundEncoder(&stalled, func() (IntraEncoder, error) { t.Error("重建了停滯後端"); return raw, nil }); err == nil {
			t.Fatal("未停用停滯後端")
		}
		close(raw.release)
		synctest.Wait()
		<-raw.closed
		r, _, _, _ := (<-raw.pixel).RGBA()
		if r != 10*257 {
			t.Fatal("原生編碼讀到已被重用的輸入")
		}
	})
}

func TestBoundedDecoderStatusAndCloseDoNotWaitForDriver(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		raw := &blockedDecoder{make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan byte, 1)}
		var stalled atomic.Bool
		d, err := boundDecoder(&stalled, func() (IntraDecoder, error) { return raw, nil })
		if err != nil {
			t.Fatal(err)
		}
		data := []byte{42}
		done := make(chan error, 1)
		go func() { _, err := d.Decode(data); done <- err }()
		<-raw.entered
		if mode := d.(interface{ DecodingMode() string }).DecodingMode(); mode != "unknown" {
			t.Fatal(mode)
		}
		if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		data[0] = 99
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-raw.closed:
			t.Fatal("解碼仍執行中就釋放資源")
		default:
		}
		close(raw.release)
		synctest.Wait()
		<-raw.closed
		if <-raw.data != 42 {
			t.Fatal("原生解碼讀到被呼叫端改寫的資料")
		}
	})
}
