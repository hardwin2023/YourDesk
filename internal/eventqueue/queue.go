// Package eventqueue 提供有容量上限的有序佇列，只合併相鄰且可取代的事件。
package eventqueue

import "sync"

type Queue[T any] struct {
	mu      sync.Mutex
	items   []T
	head    int
	count   int
	ready   chan struct{}
	replace func(previous, next T) bool
}

func New[T any](capacity int, replace func(previous, next T) bool) *Queue[T] {
	if capacity < 1 {
		panic("事件佇列容量必須大於零")
	}
	return &Queue[T]{items: make([]T, capacity), ready: make(chan struct{}, 1), replace: replace}
}

// Push 不等待消費者。limit 為零時使用完整容量；合併不增加長度，
// 即使已達 limit 仍可更新尾端，避免忙碌時保留舊位置、丟棄最新位置。
func (q *Queue[T]) Push(value T, limit int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.count > 0 && q.replace != nil {
		tail := (q.head + q.count - 1) % len(q.items)
		if q.replace(q.items[tail], value) {
			q.items[tail] = value
			return true
		}
	}
	if q.count == len(q.items) || (limit > 0 && q.count >= limit) {
		return false
	}
	q.items[(q.head+q.count)%len(q.items)] = value
	q.count++
	select {
	case q.ready <- struct{}{}:
	default:
	}
	return true
}

// Pop 供單一工作者使用；取消後不繼續執行排隊中的事件。
func (q *Queue[T]) Pop(done <-chan struct{}) (T, bool) {
	var zero T
	for {
		select {
		case <-done:
			return zero, false
		default:
		}
		q.mu.Lock()
		if q.count > 0 {
			value := q.items[q.head]
			q.items[q.head] = zero
			q.head = (q.head + 1) % len(q.items)
			q.count--
			q.mu.Unlock()
			return value, true
		}
		q.mu.Unlock()
		select {
		case <-done:
			return zero, false
		case <-q.ready:
		}
	}
}

func (q *Queue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.count
}

// Reset 原子取消尚未消費的工作，並以不可略過的重設屏障取代。
// 已由消費者取得的工作仍先完成；之後 Push 的事件排在屏障後。
func (q *Queue[T]) Reset(value T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	clear(q.items)
	q.head, q.count = 0, 1
	q.items[0] = value
	select {
	case q.ready <- struct{}{}:
	default:
	}
}
