package winmedia

import (
	"image"
	"image/color"
	"testing"
)

func TestOwnedPixelsCopiesSubimageWithPaddedStride(t *testing.T) {
	original := image.NewRGBA(image.Rect(10, 20, 18, 27))
	for y := 20; y < 27; y++ {
		for x := 10; x < 18; x++ {
			original.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	src := original.SubImage(image.Rect(12, 22, 16, 25)).(*image.RGBA)
	if !validRGBA(src) {
		t.Fatal("拒絕有效的有填補列寬之子圖")
	}
	copy := ownedRGBA(src)
	if copy.Bounds() != image.Rect(0, 0, 4, 3) {
		t.Fatal(copy.Bounds())
	}
	for y := range 3 {
		for x := range 4 {
			if copy.RGBAAt(x, y) != src.RGBAAt(x+12, y+22) {
				t.Fatal("子圖像素複製錯誤")
			}
		}
	}
	original.SetRGBA(12, 22, color.RGBA{})
	if copy.RGBAAt(0, 0).A != 255 {
		t.Fatal("原生工作者仍共用呼叫端輸入")
	}
	copy.SetRGBA(1, 1, color.RGBA{})
	if original.RGBAAt(13, 23).A != 255 {
		t.Fatal("原生工作者覆寫呼叫端輸入")
	}
}

func TestRejectInvalidPixels(t *testing.T) {
	for _, src := range []*image.RGBA{nil, {}, {Rect: image.Rect(0, 0, 2, 2), Stride: 8, Pix: make([]byte, 15)}, {Rect: image.Rect(0, 0, 2, 2), Stride: 4, Pix: make([]byte, 16)}} {
		if validRGBA(src) {
			t.Fatal("接受無效的像素緩衝")
		}
	}
}
