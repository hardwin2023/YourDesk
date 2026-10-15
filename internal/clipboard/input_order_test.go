package clipboard

import (
	"context"
	"reflect"
	"testing"
	"testing/synctest"
	"time"
	"yourdesk/internal/p2p"
	"yourdesk/internal/rawkey"
)

// 在送出工作者尚未執行的情境下，滑鼠仍須等待前面的修飾鍵，不得走旁路。
func TestModifierPointerQueueOrder(t *testing.T) {
	s := &Sync{ctx: context.Background(), peer: &p2p.Peer{}, keys: newInputQueue()}
	events := []p2p.Control{
		{Type: "raw-key", RawKey: &rawkey.Event{Platform: "darwin", Code: 56, Modifiers: 1, Down: true}},
		{Type: "button", Button: 1, Down: true, X: .2, Y: .3},
		{Type: "move", X: .3, Y: .4},
		{Type: "wheel", Delta: 1},
		{Type: "button", Button: 1, Down: false, X: .3, Y: .4},
		{Type: "raw-key", RawKey: &rawkey.Event{Platform: "darwin", Code: 56, Down: false}},
	}
	for _, e := range events {
		if err := s.SendControl(e); err != nil {
			t.Fatal(err)
		}
	}
	for _, expected := range events {
		if s.keys.Len() == 0 {
			t.Fatal("滑鼠未經有序交接")
		}
		job, _ := s.keys.Pop(context.Background().Done())
		if !reflect.DeepEqual(job.control, expected) {
			t.Fatalf("輸入順序或內容變更：%+v", job.control)
		}
	}
	for i := 0; i < 5000; i++ {
		if err := s.SendControl(p2p.Control{Type: "move", X: .5}); err != nil {
			t.Fatal(err)
		}
	}
	if s.keys.Len() != 1 {
		t.Fatal("移動事件持續累積")
	}
	if err := s.SendControl(p2p.Control{Type: "button", Button: 1, Down: false}); err != nil {
		t.Fatal("移動事件排擠滑鼠釋放")
	}
	if err := s.SendControl(p2p.Control{Type: "key", Key: "shift", Down: false}); err != nil {
		t.Fatal("移動事件排擠修飾鍵釋放")
	}
}

func TestPointerCoalescingDoesNotCrossPasteOrCancellation(t *testing.T) {
	s := &Sync{ctx: context.Background(), peer: &p2p.Peer{}, keys: newInputQueue()}
	move := func(x float64) {
		t.Helper()
		if err := s.SendControl(p2p.Control{Type: "move", X: x}); err != nil {
			t.Fatal(err)
		}
	}
	move(.1)
	move(.2)
	s.Poll(true)
	move(.3)
	move(.4)
	if s.keys.Len() != 3 {
		t.Fatal("移動跨過貼上屏障合併", s.keys.Len())
	}
	s.CancelKeys("失焦")
	move(.5)
	move(.6)
	if s.keys.Len() != 2 {
		t.Fatal("取消未清除舊輸入，或移動跨過重設屏障", s.keys.Len())
	}
	want := []keyJob{
		{generation: 1, reset: true},
		{generation: 1, control: p2p.Control{Type: "move", X: .6}},
	}
	for _, expected := range want {
		job, _ := s.keys.Pop(nil)
		if !reflect.DeepEqual(job, expected) {
			t.Fatalf("屏障／取消後的輸入內容不符：got=%+v want=%+v", job, expected)
		}
	}
}

func TestBlockedInputSendDoesNotBlockFocusCancellation(t *testing.T) {
	for _, down := range []p2p.Control{
		{Type: "key", Key: "shift", Down: true},
		{Type: "button", Button: 1, Down: true},
		{Type: "raw-key", RawKey: &rawkey.Event{Platform: "windows", Code: 42, Down: true, Modifiers: 1}},
	} {
		t.Run(down.Type, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s := &Sync{ctx: ctx, peer: &p2p.Peer{}, keys: newInputQueue()}
				started, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
				var sent []p2p.Control
				go func() {
					defer close(stopped)
					s.keyboardWithSender(ctx, func(c p2p.Control) error {
						if len(sent) == 0 {
							close(started)
							<-release
						}
						sent = append(sent, c)
						return nil
					})
				}()
				if err := s.SendControl(down); err != nil {
					t.Fatal(err)
				}
				<-started
				uiDone := make(chan struct{})
				go func() {
					defer close(uiDone)
					for range 5000 {
						_ = s.SendControl(p2p.Control{Type: "move", X: .2})
					}
					s.Poll(true)
					s.CancelKeys("失焦")
					_ = s.SendControl(p2p.Control{Type: "move", X: .9})
				}()
				select {
				case <-uiDone:
				case <-time.After(time.Second):
					close(release)
					t.Fatal("傳輸阻塞時 UI 無法排入輸入／取消")
				}
				if s.keys.Len() != 2 {
					t.Fatal("失焦後仍累積過期事件", s.keys.Len())
				}
				close(release)
				synctest.Wait()
				if len(sent) != 4 || !reflect.DeepEqual(sent[0], down) || sent[1].Type != down.Type || sent[2].Type != "raw-key-reset" || sent[3].X != .9 {
					t.Fatalf("傳送中的按下事件未先釋放再送出新輸入：%+v", sent)
				}
				if sent[1].Down || (sent[1].RawKey != nil && (sent[1].RawKey.Down || sent[1].RawKey.Modifiers != 0)) {
					t.Fatal("重設未釋放按鍵")
				}
				cancel()
				<-stopped
			})
		})
	}
}

func TestExplicitRawResetIsQueuedEvenWithoutHeldKeys(t *testing.T) {
	for _, direct := range []bool{false, true} {
		s := &Sync{ctx: context.Background(), peer: &p2p.Peer{}, keys: newInputQueue()}
		if direct {
			s.ResetKeys("原始鍵盤失焦")
		} else if err := s.SendControl(p2p.Control{Type: "raw-key-reset"}); err != nil {
			t.Fatal(err)
		}
		job, ok := s.keys.Pop(nil)
		if !ok || !job.reset {
			t.Fatal("閒置時略過了明確要求的重設")
		}
	}
}
