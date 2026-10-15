package core

import (
	"context"
	"testing"
	"testing/synctest"
	"yourdeskandroid/core/internal/p2p"
)

func TestAndroidInputRemainsResponsiveDuringBlockedSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		entered, release := make(chan struct{}), make(chan struct{})
		var sent []p2p.Control
		s := newInputSender(ctx, func(c p2p.Control) error {
			if len(sent) == 0 {
				close(entered)
				<-release
			}
			sent = append(sent, c)
			return nil
		})
		if err := s.enqueue(p2p.Control{Type: "button", Button: 1, Down: true}); err != nil {
			t.Fatal(err)
		}
		<-entered
		for range 5000 {
			if err := s.enqueue(p2p.Control{Type: "move", X: .2}); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.enqueue(p2p.Control{Type: "key", Key: "shift", Down: true}); err != nil {
			t.Fatal(err)
		}
		if err := s.enqueue(p2p.Control{Type: "raw-key-reset"}); err != nil {
			t.Fatal(err)
		}
		if err := s.enqueue(p2p.Control{Type: "move", X: .9}); err != nil {
			t.Fatal(err)
		}
		if s.count != 2 {
			t.Fatal("取消後仍保留過期輸入", s.count)
		}
		close(release)
		synctest.Wait()
		if len(sent) != 4 || sent[0].Type != "button" || !sent[0].Down || sent[1].Type != "button" || sent[1].Down || sent[2].Type != "raw-key-reset" || sent[3].X != .9 {
			t.Fatal("重設未先釋放已傳送按鈕，或重播已取消輸入", sent)
		}
		cancel()
		synctest.Wait()
		if err := s.enqueue(p2p.Control{Type: "move"}); err == nil {
			t.Fatal("斷線後仍接受輸入")
		}
	})
}

func TestAndroidVideoQueueHasTwoSlotsAndRecoversFromIDR(t *testing.T) {
	v := NewViewer()
	for seq := uint64(1); seq <= 3; seq++ {
		v.enqueueFrame(p2p.Frame{Sequence: seq, Codec: 2, Keyframe: seq == 1, JPEG: []byte{1}})
	}
	if v.frameCount != 0 || !v.ConsumeFrameRecoveryRequest() {
		t.Fatal("影片滿載仍累積相依幀")
	}
	v.enqueueFrame(p2p.Frame{Sequence: 4, Codec: 2, JPEG: []byte{1}})
	if v.frameCount != 0 {
		t.Fatal("缺少參考影格後仍接受 P frame")
	}
	v.enqueueFrame(p2p.Frame{Sequence: 5, Codec: 2, Keyframe: true, JPEG: []byte{1}})
	if f := v.popFrame(); f == nil || f.Sequence != 5 {
		t.Fatal("新的 IDR 未恢復串流")
	}
	// JPEG 區塊數量沿用原來的上限，不將同張桌面的多個 patch 當成影片積壓。
	v = NewViewer()
	for seq := uint64(1); seq <= 20; seq++ {
		v.enqueueFrame(p2p.Frame{Sequence: seq, Keyframe: seq == 1, JPEG: []byte{1}})
	}
	if v.frameCount != 20 {
		t.Fatal("誤刪 JPEG 區塊")
	}
}
