package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"yourdesk/internal/p2p"
)

//go:embed fixtures/video-*-1080.json
var videoFixtures embed.FS

type videoPacket struct {
	Keyframe bool   `json:"keyframe"`
	Data     []byte `json:"data"`
}
type videoFixture struct {
	Codec  byte          `json:"codec"`
	Width  int           `json:"width"`
	Height int           `json:"height"`
	Frames []videoPacket `json:"frames"`
}

// 在真實 P2P 上重播包含 IDR／相依幀的合成 GOP；只供 androidTest 使用。
type videoSmoke struct {
	mu             sync.Mutex
	fixtures       map[byte]videoFixture
	codec          byte
	index          int
	drop           bool
	sent, requests uint64
}

func newVideoSmoke() *videoSmoke {
	v := &videoSmoke{fixtures: make(map[byte]videoFixture)}
	for _, name := range []string{"h264", "hevc"} {
		data, err := videoFixtures.ReadFile("fixtures/video-" + name + "-1080.json")
		if err != nil {
			panic(err)
		}
		var f videoFixture
		if err := json.Unmarshal(data, &f); err != nil {
			panic(err)
		}
		if len(f.Frames) != 60 || !f.Frames[0].Keyframe || f.Frames[1].Keyframe {
			panic("無效合成 GOP")
		}
		v.fixtures[f.Codec] = f
	}
	return v
}

func (v *videoSmoke) control(c p2p.Control) {
	if c.Type != "video-capabilities" {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.codec == 0 {
		return
	}
	for _, codec := range c.Codecs {
		if codec == v.codec {
			return
		}
	}
	v.codec, v.index = 0, 0
}

func (v *videoSmoke) attach(peer *p2p.Peer) {
	register := func(name string, run func() any) {
		if err := peer.RegisterCommand(name, func(context.Context) (any, error) {
			v.mu.Lock()
			defer v.mu.Unlock()
			return run(), nil
		}); err != nil {
			panic(err)
		}
	}
	for codec, name := range map[byte]string{0: "jpeg", 1: "h264", 2: "hevc"} {
		register("smoke.video."+name, func() any {
			v.codec, v.index, v.drop = codec, 0, false
			v.sent, v.requests = 0, 0
			return map[string]byte{"codec": codec}
		})
	}
	register("smoke.video.drop", func() any { v.drop = true; return nil })
	register("smoke.video.status", func() any {
		return map[string]any{"codec": v.codec, "sent": v.sent, "keyframeRequests": v.requests}
	})
	register("video.keyframe", func() any {
		v.index = 0
		v.requests++
		return map[string]bool{"accepted": true}
	})
}

func (v *videoSmoke) send(peer *p2p.Peer, sequence uint64) bool {
	v.mu.Lock()
	if v.codec == 0 {
		v.mu.Unlock()
		return false
	}
	f := v.fixtures[v.codec]
	p := f.Frames[v.index]
	v.index = (v.index + 1) % len(f.Frames)
	skip := v.drop && !p.Keyframe
	if skip {
		v.drop = false
	}
	v.mu.Unlock()
	if skip {
		return true
	} // 留下序號缺口，驗證接收端要求 IDR 並恢復。
	current := 0
	_ = peer.SendControl(p2p.Control{Type: "displays", Display: &current, DisplayCount: 1})
	err := peer.SendFrame(p2p.Frame{Sequence: sequence, Display: 0, Codec: f.Codec,
		Width: uint32(f.Width), Height: uint32(f.Height), Keyframe: p.Keyframe, JPEG: p.Data})
	v.mu.Lock()
	if err == nil {
		v.sent++
	} else {
		v.index = 0
	}
	v.mu.Unlock()
	return true
}

func (v *videoSmoke) String() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return fmt.Sprintf("codec=%d sent=%d keyframeRequests=%d", v.codec, v.sent, v.requests)
}
