package video

import (
	_ "embed"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"sync"
	"yourdesk/internal/optimization"
)

// WireCodec 使用影格標頭的保留位元組；0 保留給舊版 JPEG。
type WireCodec byte

const (
	WireJPEG WireCodec = iota
	WireH264
	WireHEVC
	WireAV1
)

// IntraEncoder 保留既有名稱；協商後可輸出含參考影格的 GOP。
type IntraEncoder interface {
	Encode(image.Image, int) ([]byte, error)
	Close() error
}

// GOPEncoder 設定 IDR 間隔，預設 1 以相容舊 遠端顯示。
type GOPEncoder interface{ SetKeyframeInterval(int) }

// RateEncoder 設定平均位元率及影格時間；零位元率使用編碼器預設值。
type RateEncoder interface {
	SetRate(bitsPerSecond, fps int)
}

var ErrVideoUnavailable = errors.New("硬體影像編碼不可用")

type IntraCapability struct {
	Codec       Codec  `json:"codec"`
	Encode      bool   `json:"encode"`
	Decode      bool   `json:"decode"`
	DecodeMode  string `json:"decodeMode,omitempty"`
	EncodeError string `json:"encodeError,omitempty"`
	DecodeError string `json:"decodeError,omitempty"`
}

var intraOnce sync.Once
var intraCaps []IntraCapability
var intraReady = make(chan struct{})

// 探測用固定黑色影格由 FFmpeg 產生，解碼能力不依賴本機編碼器。
//
//go:embed probes/h264.bin
var probeH264 []byte

//go:embed probes/hevc.bin
var probeHEVC []byte

//go:embed probes/av1-128.obu
var probeAV1 []byte

func startIntraProbe() {
	intraOnce.Do(func() { go probeIntraCapabilities() })
}

// UI 僅讀取已完成的快取，首次呼叫啟動背景探測。
func CachedIntraCapabilities() []IntraCapability {
	startIntraProbe()
	select {
	case <-intraReady:
		return append([]IntraCapability(nil), intraCaps...)
	default:
		return nil
	}
}
func IntraCapabilities() []IntraCapability {
	startIntraProbe()
	<-intraReady
	return append([]IntraCapability(nil), intraCaps...)
}
func probeIntraCapabilities() {
	defer close(intraReady)
	startDecodeProbe()
	<-decodeReady
	policy := optimization.Snapshot()
	caps := append([]IntraCapability(nil), decodeCaps...)
	for _, cap := range decodeCaps {
		if cap.Codec == CodecHardwareAV1 {
			cap.Codec = CodecSoftwareAV1
			caps = append(caps, cap)
		}
	}
	for _, cap := range caps {
		codec := cap.Codec
		usable, err := probeEncoderCapability(codec, policy, NewIntraEncoder)
		cap.Encode = usable
		if err != nil {
			cap.EncodeError = err.Error()
		}
		if !cap.Encode {
			slog.Warn("影片編碼能力探測未通過", "codec", codec, "error", cap.EncodeError)
		}
		if !cap.Decode {
			slog.Warn("影片解碼能力探測未通過", "codec", codec, "error", cap.DecodeError)
		}
		intraCaps = append(intraCaps, cap)
	}
}

// 128×128 與 1080p 分別驗證；小影格失敗不能抹去另一個正式尺寸的成功證據。
// 是否可用仍由正式 RGBA 編碼結果決定，不把原生診斷矩陣直接當成串流成功。
func probeEncoderCapability(codec Codec, policy optimization.Policy, create func(Codec) (IntraEncoder, error)) (bool, error) {
	key := decodePolicyCodec(WireForCodec(codec))
	if codec == CodecSoftwareAV1 {
		key = "software-av1"
	}
	sizes := []image.Point{{X: 128, Y: 128}, {X: 1920, Y: 1080}}
	for _, size := range sizes {
		if usable, known := policy.EncoderDecision(key, size.X, size.Y); known && usable {
			return true, nil
		}
	}
	var failures []error
	for _, size := range sizes {
		if _, known := policy.EncoderDecision(key, size.X, size.Y); known {
			failures = append(failures, fmt.Errorf("%d×%d 正式編碼實測未通過", size.X, size.Y))
			continue
		}
		enc, err := create(codec)
		if err != nil {
			return false, err
		}
		pixels := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
		for i := 3; i < len(pixels.Pix); i += 4 {
			pixels.Pix[i] = 255
		}
		payload, encodeErr := enc.Encode(pixels, 70)
		if encodeErr == nil && len(payload) == 0 {
			encodeErr = ErrVideoUnavailable
		}
		if encodeErr == nil {
			switch codec {
			case CodecHardwareH264:
				_, encodeErr = unpackH264(payload)
			case CodecHardwareHEVC:
				_, encodeErr = unpackHEVC(payload)
			case CodecHardwareAV1, CodecSoftwareAV1:
				_, encodeErr = IsKeyframe(WireAV1, payload)
			}
		}
		if encodeErr == nil {
			if reporter, ok := enc.(BackendReporter); ok {
				slog.Info("影片編碼能力探測通過", "codec", codec, "width", size.X, "height", size.Y, "backend", reporter.Backend())
			}
		}
		enc.Close()
		if encodeErr == nil {
			return true, nil
		}
		failures = append(failures, fmt.Errorf("%d×%d：%w", size.X, size.Y, encodeErr))
	}
	return false, errors.Join(failures...)
}
func WireForCodec(codec Codec) WireCodec {
	if codec == CodecHardwareAV1 || codec == CodecSoftwareAV1 {
		return WireAV1
	}
	if codec == CodecHardwareH264 {
		return WireH264
	}
	if codec == CodecHardwareHEVC {
		return WireHEVC
	}
	return WireJPEG
}
func SupportsIntra(codec Codec) bool {
	for _, cap := range IntraCapabilities() {
		if cap.Codec == codec {
			return cap.Encode
		}
	}
	return false
}
