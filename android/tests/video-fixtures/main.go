// 以正式桌面編碼器產生合成 GOP，供手機測試實際硬體解碼；不含使用者畫面。
package main

import (
	"encoding/json"
	"flag"
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"
	"path/filepath"

	"yourdesk/internal/video"
)

type packet struct {
	Keyframe bool   `json:"keyframe"`
	Data     []byte `json:"data"`
}

type fixture struct {
	Codec  byte     `json:"codec"`
	Width  int      `json:"width"`
	Height int      `json:"height"`
	FPS    int      `json:"fps"`
	Frames []packet `json:"frames"`
}

func main() {
	dir := flag.String("out", "android/tests/smoke-host/fixtures", "測試素材輸出目錄")
	flag.Parse()
	for _, spec := range []struct {
		name  string
		codec video.Codec
	}{{"h264", video.CodecHardwareH264}, {"hevc", video.CodecHardwareHEVC}} {
		enc, err := video.NewIntraEncoder(spec.codec)
		if err != nil {
			log.Fatal(err)
		}
		enc.(video.GOPEncoder).SetKeyframeInterval(30)
		enc.(video.RateEncoder).SetRate(4_000_000, 60)
		f := fixture{Codec: byte(video.WireForCodec(spec.codec)), Width: 1920, Height: 1080, FPS: 60}
		pixels := image.NewRGBA(image.Rect(0, 0, f.Width, f.Height))
		for i := 0; i < 60; i++ {
			draw.Draw(pixels, pixels.Bounds(), image.NewUniform(color.RGBA{0, 240, 0, 255}), image.Point{}, draw.Src)
			draw.Draw(pixels, image.Rect(20, 20, 1900, 1060), image.NewUniform(color.RGBA{20, 55, 90, 255}), image.Point{}, draw.Src)
			x := 40 + i*25
			draw.Draw(pixels, image.Rect(x, 120, x+200, 960), image.NewUniform(color.RGBA{240, 180, 30, 255}), image.Point{}, draw.Src)
			payload, err := enc.Encode(pixels, 80)
			if err != nil {
				log.Fatal(err)
			}
			key, err := video.IsKeyframe(video.WireCodec(f.Codec), payload)
			if err != nil {
				log.Fatal(err)
			}
			f.Frames = append(f.Frames, packet{key, payload})
		}
		if err := enc.Close(); err != nil {
			log.Fatal(err)
		}
		if !f.Frames[0].Keyframe || f.Frames[1].Keyframe {
			log.Fatal("素材必須包含 IDR 與相依幀")
		}
		encoded, err := json.Marshal(f)
		if err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(*dir, "video-"+spec.name+"-1080.json")
		if err := os.WriteFile(path, append(encoded, '\n'), 0644); err != nil {
			log.Fatal(err)
		}
		log.Printf("%s：%d 幀，%d bytes", path, len(f.Frames), len(encoded))
	}
}
