package p2p

import (
	"sync/atomic"
	"time"
	"yourdesk/internal/eventqueue"
)

// 只追蹤正在處理的事件；閒置不是停滯。每個工作者只有一個監視器，
// 不為每次原生呼叫建立 goroutine，也不在逾時後重用仍被持有的資源。
func runControlWorker(done <-chan struct{}, inbox *eventqueue.Queue[Control], handler func(Control), stalled func()) {
	var started atomic.Pointer[time.Time]
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-finished:
				return
			case <-ticker.C:
				at := started.Load()
				if at != nil && time.Since(*at) >= 30*time.Second && started.CompareAndSwap(at, nil) {
					stalled()
					return
				}
			}
		}
	}()
	for {
		c, ok := inbox.Pop(done)
		if !ok {
			return
		}
		select {
		case <-done:
			return
		default:
		}
		now := time.Now()
		started.Store(&now)
		handler(c)
		started.Store(nil)
	}
}
