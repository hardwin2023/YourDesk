package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestViewerConnectionCleanupWaitsAndIsIdempotent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var cleanups atomic.Int32
		c := &viewerConnection{cancel: cancel, done: make(chan struct{}), cleanup: func() { cleanups.Add(1) }}
		release := make(chan struct{})
		c.workers.Add(1)
		go func() { defer c.workers.Done(); <-release }()
		c.Close()
		c.Close()
		synctest.Wait()
		if ctx.Err() == nil {
			t.Fatal("session not cancelled")
		}
		select {
		case <-c.done:
			t.Fatal("freed resources while worker active")
		default:
		}
		close(release)
		synctest.Wait()
		select {
		case <-c.done:
		default:
			t.Fatal("cleanup did not finish")
		}
		if cleanups.Load() != 1 {
			t.Fatalf("cleanups=%d", cleanups.Load())
		}
	})
}

func TestViewerShutdownStopsWaitingWithoutFreeingBusyDecoder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, cancel := context.WithCancel(context.Background())
		release := make(chan struct{})
		var cleaned atomic.Bool
		c := &viewerConnection{cancel: cancel, done: make(chan struct{}), cleanup: func() { <-release; cleaned.Store(true) }}
		ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		if err := c.CloseAndWait(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		if cleaned.Load() {
			t.Fatal("關閉逾時釋放了仍在使用的資源")
		}
		close(release)
		if err := c.CloseAndWait(context.Background()); err != nil || !cleaned.Load() {
			t.Fatal("原工作者返回後未回收資源", err)
		}
	})
}
