package ownedworker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestStalledCallRetiresWithoutFreeingActiveResource(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var destroyed atomic.Bool
		w, err := New[int, int](context.Background(), func() (int, error) { return 42, nil }, func(int) { destroyed.Store(true) })
		if err != nil {
			t.Fatal(err)
		}
		started, release := make(chan struct{}), make(chan struct{})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		result := make(chan error, 1)
		go func() {
			_, err := w.Call(ctx, func(resource int) int {
				close(started)
				<-release
				if destroyed.Load() {
					t.Error("操作尚未完成就釋放原生資源")
				}
				return resource
			})
			result <- err
		}()
		<-started
		if err := <-result; !errors.Is(err, ErrStalled) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		if destroyed.Load() {
			t.Fatal("逾時強制釋放了資源")
		}
		for range 100 {
			if _, err := w.Call(context.Background(), func(int) int { t.Error("退役後仍接受操作"); return 0 }); !errors.Is(err, ErrStalled) {
				t.Fatal(err)
			}
		}
		close(release)
		synctest.Wait()
		if !destroyed.Load() {
			t.Fatal("原生呼叫返回後沒有回收資源")
		}
		if err := w.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCreationAndDestructionHaveBoundedWaits(t *testing.T) {
	for _, phase := range []string{"create", "destroy"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				release := make(chan struct{})
				var destroyed atomic.Bool
				create := func() (int, error) {
					if phase == "create" {
						<-release
					}
					return 1, nil
				}
				destroy := func(int) {
					if phase == "destroy" {
						<-release
					}
					destroyed.Store(true)
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				w, err := New[int, int](ctx, create, destroy)
				if phase == "destroy" {
					if err != nil {
						t.Fatal(err)
					}
					err = w.Close(ctx)
				}
				if !errors.Is(err, ErrStalled) {
					t.Fatalf("%s 未在期限內返回：%v", phase, err)
				}
				if destroyed.Load() {
					t.Fatal("阻塞中的資源已被提前回收")
				}
				close(release)
				synctest.Wait()
				if !destroyed.Load() {
					t.Fatal("解除阻塞後未完成回收")
				}
			})
		})
	}
}

func TestCallsAreSerializedAndCloseCancelsWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var active atomic.Int32
		w, err := New[int, int](context.Background(), func() (int, error) { return 7, nil }, func(int) {
			if active.Load() != 0 {
				t.Error("銷毀與操作同時執行")
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		started, release := make(chan struct{}), make(chan struct{})
		results := make(chan error, 2)
		go func() {
			_, err := w.Call(context.Background(), func(int) int { active.Add(1); close(started); <-release; active.Add(-1); return 1 })
			results <- err
		}()
		<-started
		go func() {
			_, err := w.Call(context.Background(), func(int) int { t.Error("關閉後仍執行等待中的操作"); return 0 })
			results <- err
		}()
		synctest.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := w.Close(ctx); !errors.Is(err, ErrStalled) {
			t.Fatal(err)
		}
		for range 2 {
			if err := <-results; !errors.Is(err, ErrClosed) {
				t.Fatal(err)
			}
		}
		close(release)
		if err := w.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSuccessfulCallsAndFactoryError(t *testing.T) {
	factoryErr := errors.New("無法建立")
	if _, err := New[int, int](context.Background(), func() (int, error) { return 0, factoryErr }, func(int) { t.Error("失敗的資源不可銷毀") }); !errors.Is(err, factoryErr) {
		t.Fatal(err)
	}
	w, err := New[int, int](context.Background(), func() (int, error) { return 9, nil }, func(int) {})
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if got, err := w.Call(context.Background(), func(resource int) int { return resource + 1 }); err != nil || got != 10 {
			t.Fatal(got, err)
		}
	}
	if err := w.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
