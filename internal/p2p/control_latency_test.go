package p2p

import (
	"reflect"
	"testing"
	"testing/synctest"
)

func TestBusyControlWorkerUsesLatestPointerAndPreservesInputOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &Peer{done: make(chan struct{})}
		defer p.markClosed()
		release := make(chan struct{})
		out := make(chan Control, 16)
		handler := func(c Control) {
			out <- c
			if c.Type == "key" && c.Down {
				<-release
			}
		}
		first := Control{Type: "key", Key: "shift", Down: true}
		p.dispatchControl(first, handler)
		synctest.Wait()
		for i := 1; i <= 5000; i++ {
			p.dispatchControl(Control{Type: "move", X: float64(i) / 5000, ViewID: 1}, handler)
		}
		down := Control{Type: "button", Button: 1, Down: true, X: 1, ViewID: 1}
		p.dispatchControl(down, handler)
		for i := 1; i <= 5000; i++ {
			p.dispatchControl(Control{Type: "move", X: 1, Y: float64(i) / 5000, ViewID: 1}, handler)
		}
		up := down
		up.Down, up.Y = false, 1
		p.dispatchControl(up, handler)
		last := Control{Type: "key", Key: "shift", Down: false}
		p.dispatchControl(last, handler)
		if p.controlInbox.Len() != 5 {
			t.Fatal("滑鼠位置仍大量堆積，按鍵必須等待舊移動", p.controlInbox.Len())
		}
		close(release)
		synctest.Wait()
		want := []Control{first, {Type: "move", X: 1, ViewID: 1}, down, {Type: "move", X: 1, Y: 1, ViewID: 1}, up, last}
		var got []Control
		for len(out) > 0 {
			got = append(got, <-out)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("最新位置、拖曳或按鍵順序不正確", got)
		}
	})
}

func TestMoveCoalescingRespectsPointerView(t *testing.T) {
	a, b := 0, 1
	first := Control{Type: "move", ViewID: 1, Display: &a, Platform: "windows"}
	same := first
	same.Display = new(int) // 比較螢幕值，不能比較指標位址。
	if !CanCoalesceMoves(first, same) {
		t.Fatal("同視野無法合併")
	}
	for _, change := range []func(*Control){
		func(c *Control) { c.Type = "button" },
		func(c *Control) { c.ViewID++ },
		func(c *Control) { c.Display = &b },
		func(c *Control) { c.Display = nil },
		func(c *Control) { c.Platform = "darwin" },
		func(c *Control) { c.DisableMapping = true },
	} {
		next := first
		change(&next)
		if CanCoalesceMoves(first, next) || CanCoalesceMoves(next, first) {
			t.Fatal("不同事件或視野被合併", next)
		}
	}
}
