package video

import (
	"image"
	"testing"
	"yourdesk/internal/optimization"
)

type sizeProbeEncoder struct {
	attempts *[]image.Point
	closed   *int
	valid    bool
}

func (e *sizeProbeEncoder) Encode(img image.Image, _ int) ([]byte, error) {
	size := img.Bounds().Size()
	*e.attempts = append(*e.attempts, size)
	if size.X == 128 {
		return nil, ErrVideoUnavailable
	}
	if !e.valid {
		return []byte{1, 2, 3}, nil
	}
	return probeHEVC, nil
}
func (e *sizeProbeEncoder) Close() error { *e.closed++; return nil }

func TestEncoderCapabilitiesAreNotLimitedToSmallFrames(t *testing.T) {
	for _, valid := range []bool{false, true} {
		var attempts []image.Point
		closed := 0
		usable, err := probeEncoderCapability(CodecHardwareHEVC, optimization.Policy{}, func(codec Codec) (IntraEncoder, error) {
			if codec != CodecHardwareHEVC {
				t.Fatal("編碼器種類錯誤")
			}
			return &sizeProbeEncoder{&attempts, &closed, valid}, nil
		})
		if usable != valid || (err != nil) == valid || len(attempts) != 2 || attempts[1] != (image.Point{X: 1920, Y: 1080}) || closed != 2 {
			t.Fatalf("尺寸或正式封包驗證錯誤：usable=%v err=%v attempts=%v closed=%d", usable, err, attempts, closed)
		}
	}
}

func TestEncoderCapabilityUsesIndependentSizeEvidence(t *testing.T) {
	for _, codec := range []Codec{CodecHardwareHEVC, CodecSoftwareAV1} {
		key := "hevc"
		if codec == CodecSoftwareAV1 {
			key = "software-av1"
		}
		policy := optimization.Policy{Encoders: []optimization.Encoder{
			{Codec: key, Width: 128, Height: 128, Usable: false},
			{Codec: key, Width: 1920, Height: 1080, Usable: true},
		}}
		create := func(Codec) (IntraEncoder, error) {
			t.Fatal("已有正式證據不應重新建立硬體工作階段")
			return nil, nil
		}
		usable, err := probeEncoderCapability(codec, policy, create)
		if err != nil || !usable {
			t.Fatal("小尺寸失敗抹去了 1080p 成功證據", err)
		}
		policy.Encoders[1].Usable = false
		usable, err = probeEncoderCapability(codec, policy, create)
		if usable || err == nil {
			t.Fatal("已知失敗被誤標為可用")
		}
	}
}
