package core

import (
	"context"
	"fmt"
	"sync"
	"yourdeskandroid/core/internal/p2p"
)

// 每次連線只有一個傳送者；UI 不持有任何傳輸鎖。僅合併尾端同螢幕的
// 相鄰移動，可靠事件維持順序，重設先清除舊工作再釋放實際送出的按鍵。
type inputSender struct {
	ctx         context.Context
	mu          sync.Mutex
	items       [4096]p2p.Control
	head, count int
	ready       chan struct{}
}

func newInputSender(ctx context.Context, send func(p2p.Control) error) *inputSender {
	s := &inputSender{ctx: ctx, ready: make(chan struct{}, 1)}
	go s.run(send)
	return s
}
func (s *inputSender) enqueue(c p2p.Control) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if c.Type == "raw-key-reset" {
		clear(s.items[:])
		s.head, s.count = 0, 0
	}
	if s.count > 0 && c.Type == "move" {
		tail := (s.head + s.count - 1) % len(s.items)
		previous := s.items[tail]
		sameDisplay := previous.Display == nil && c.Display == nil || previous.Display != nil && c.Display != nil && *previous.Display == *c.Display
		if previous.Type == "move" && sameDisplay && previous.Platform == c.Platform && previous.DisableMapping == c.DisableMapping {
			s.items[tail] = c
			return nil
		}
	}
	if s.count == len(s.items) {
		return fmt.Errorf("輸入佇列已滿")
	}
	if c.Type == "move" && s.count >= 64 {
		return nil
	}
	s.items[(s.head+s.count)%len(s.items)] = c
	s.count++
	select {
	case s.ready <- struct{}{}:
	default:
	}
	return nil
}
func (s *inputSender) pop() (p2p.Control, bool) {
	for s.ctx.Err() == nil {
		s.mu.Lock()
		if s.count > 0 {
			c := s.items[s.head]
			s.items[s.head] = p2p.Control{}
			s.head = (s.head + 1) % len(s.items)
			s.count--
			s.mu.Unlock()
			return c, true
		}
		s.mu.Unlock()
		select {
		case <-s.ctx.Done():
			return p2p.Control{}, false
		case <-s.ready:
		}
	}
	return p2p.Control{}, false
}
func (s *inputSender) run(send func(p2p.Control) error) {
	held := make(map[string]p2p.Control)
	defer func() { s.mu.Lock(); clear(s.items[:]); s.count = 0; s.mu.Unlock() }()
	for {
		c, ok := s.pop()
		if !ok {
			return
		}
		if c.Type == "raw-key-reset" {
			for _, down := range held {
				down.Down = false
				if down.RawKey != nil {
					key := *down.RawKey
					key.Down, key.Repeat, key.Modifiers = false, false, 0
					down.RawKey = &key
				}
				_ = send(down)
			}
			clear(held)
			_ = send(c)
			continue
		}
		if err := send(c); err != nil {
			continue
		}
		id, down := c.Key, c.Down
		if c.RawKey != nil {
			id = fmt.Sprintf("%s/%d", c.RawKey.Platform, c.RawKey.Code)
			down = c.RawKey.Down
		}
		if c.Type == "button" {
			id = fmt.Sprintf("mouse/%d", c.Button)
		}
		if c.Type == "key" || c.Type == "raw-key" || c.Type == "button" {
			if down {
				held[id] = c
			} else {
				delete(held, id)
			}
		}
	}
}
