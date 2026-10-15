//go:build windows && cgo

package video

import (
	"image"
	"os"
	"strings"
	"testing"
)

// 可在問題 Windows 主機執行；指定 YOURDESK_REQUIRE_HW_ENCODER=hevc 時不允許跳過。
func TestWindowsHardwareEncoderSmoke(t *testing.T) {
	for _, codec := range []Codec{CodecHardwareH264, CodecHardwareHEVC} {
		t.Run(string(codec), func(t *testing.T) {
			enc, err := NewIntraEncoder(codec)
			required := strings.TrimPrefix(string(codec), "hardware-") == os.Getenv("YOURDESK_REQUIRE_HW_ENCODER")
			if err != nil {
				if required {
					t.Fatal(err)
				}
				t.Skip("此主機無可用的 MF 工作階段：", err)
			}
			defer enc.Close()
			pixels := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
			for i := 3; i < len(pixels.Pix); i += 4 {
				pixels.Pix[i] = 255
			}
			payload, err := enc.Encode(pixels, 70)
			if err != nil {
				if required {
					t.Fatal(err)
				}
				t.Skip("此主機沒有可用的硬體編碼路徑：", err)
			}
			key, err := IsKeyframe(WireForCodec(codec), payload)
			if err != nil || !key {
				t.Fatal("第一張不是有效的關鍵影格", err)
			}
			for _, rate := range []int{0, 8000000} {
				enc.(RateEncoder).SetRate(rate, 30)
				enc.(GOPEncoder).SetKeyframeInterval(10)
				for i := 0; i < 3; i++ {
					payload, err = enc.Encode(pixels, 70)
					if err != nil {
						t.Fatal("正式 GOP 編碼失敗", err)
					}
					if _, err = IsKeyframe(WireForCodec(codec), payload); err != nil {
						t.Fatal(err)
					}
				}
			}
			if reporter, ok := enc.(BackendReporter); ok {
				t.Log(reporter.Backend())
			}
		})
	}
}
