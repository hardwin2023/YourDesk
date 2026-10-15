// Package ownedworker 將原生資源限制在單一執行緒，逾時後只退役，不強制釋放仍在使用的資源。
package ownedworker

import (
	"context"
	"errors"
	"runtime"
	"sync"
)

var ErrClosed = errors.New("原生工作者已關閉")
var ErrStalled = errors.New("原生工作者未在期限內完成，已退役")

type request[R, T any] struct {
	run  func(R) T
	done chan T
}

type Worker[R, T any] struct {
	requests chan request[R, T]
	stop     chan struct{}
	done     chan struct{}
	gate     chan struct{}
	mu       sync.Mutex
	err      error
}

// 建立、操作及銷毀均在同一 OS 執行緒。create 卡住時不再接受工作；
// 它若稍後返回，仍由原工作者銷毀資源，不會和呼叫端競爭使用權。
func New[R, T any](ctx context.Context, create func() (R, error), destroy func(R)) (*Worker[R, T], error) {
	w := &Worker[R, T]{requests: make(chan request[R, T]), stop: make(chan struct{}), done: make(chan struct{}), gate: make(chan struct{}, 1)}
	w.gate <- struct{}{}
	ready := make(chan error, 1)
	go func() {
		defer close(w.done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		resource, err := create()
		ready <- err
		if err != nil {
			return
		}
		defer destroy(resource)
		for {
			select {
			case <-w.stop:
				return
			case job := <-w.requests:
				select {
				case <-w.stop:
					return
				default:
				}
				job.done <- job.run(resource)
			}
		}
	}()
	select {
	case err := <-ready:
		if err != nil {
			return nil, err
		}
		if ctx.Err() == nil {
			return w, nil
		}
	case <-ctx.Done():
	}
	err := errors.Join(ErrStalled, ctx.Err())
	w.retire(err)
	return nil, err
}

func (w *Worker[R, T]) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

func (w *Worker[R, T]) retire(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err == nil {
		w.err = err
		close(w.stop)
	}
}

// run 必須持有獨立輸入／輸出副本；即使 Call 逾時，run 仍可能在原生 API 中。
// 每個工作者只有一個執行者，不為每次呼叫建立無法回收的 goroutine。
func (w *Worker[R, T]) Call(ctx context.Context, run func(R) T) (T, error) {
	var zero T
	select {
	case <-w.stop:
		return zero, w.Err()
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-w.gate:
	}
	defer func() { w.gate <- struct{}{} }()
	if err := w.Err(); err != nil {
		return zero, err
	}
	job := request[R, T]{run: run, done: make(chan T, 1)}
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-w.stop:
		return zero, w.Err()
	case w.requests <- job:
	}
	select {
	case value := <-job.done:
		return value, nil
	case <-w.stop:
		return zero, w.Err()
	case <-ctx.Done():
		err := errors.Join(ErrStalled, ctx.Err())
		w.retire(err)
		return zero, err
	}
}

func (w *Worker[R, T]) Close(ctx context.Context) error {
	w.retire(ErrClosed)
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return errors.Join(ErrStalled, ctx.Err())
	}
}
