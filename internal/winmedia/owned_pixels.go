package winmedia

import (
	"image"
	"sync"
)

var pixelInputs sync.Pool

func validRGBA(src *image.RGBA) bool {
	if src == nil {
		return false
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w < 1 || h < 1 || w > 8192 || h > 8192 || w*h > 32<<20 || src.Stride < w*4 {
		return false
	}
	// 除法檢查避免外部提供的極大 Stride 在乘法時溢位。
	return len(src.Pix) >= w*4 && (h == 1 || src.Stride <= (len(src.Pix)-w*4)/(h-1))
}

// 原生工作者逾時後仍持有自己的副本，呼叫端可立即重用或釋放來源。
func ownedRGBA(src *image.RGBA) *image.RGBA {
	out, _ := pixelInputs.Get().(*image.RGBA)
	if out == nil {
		out = new(image.RGBA)
	}
	out.Rect = image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy())
	out.Stride = src.Bounds().Dx() * 4
	size := out.Stride * src.Bounds().Dy()
	if cap(out.Pix) < size {
		out.Pix = make([]byte, size)
	} else {
		out.Pix = out.Pix[:size]
	}
	for y := 0; y < src.Bounds().Dy(); y++ {
		copy(out.Pix[y*out.Stride:(y+1)*out.Stride], src.Pix[y*src.Stride:y*src.Stride+out.Stride])
	}
	return out
}

// 必須由原生呼叫返回後的工作者歸還，逾時的呼叫端不可提早重用。
func releaseOwnedRGBA(src *image.RGBA) { pixelInputs.Put(src) }
