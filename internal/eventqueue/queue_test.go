package eventqueue

import (
	"reflect"
	"testing"
	"testing/synctest"
)

func TestBoundedQueueMergesTailWithoutLosingDiscreteEvents(t *testing.T) {
	q := New(4, func(previous, next int) bool { return previous < 0 && next < 0 })
	for _, value := range []int{1, -1, 2, -2} {
		if !q.Push(value, 0) {
			t.Fatal("未達上限就拒絕事件")
		}
	}
	if !q.Push(-99, 2) || q.Len() != 4 || q.Push(3, 0) {
		t.Fatal("滿載時未更新尾端，或遺失先前接受的事件")
	}
	var got []int
	for q.Len() > 0 {
		value, _ := q.Pop(nil)
		got = append(got, value)
		if value == 1 && !q.Push(3, 0) {
			t.Fatal("環形佇列無法重用已釋放的位置")
		}
	}
	if !reflect.DeepEqual(got, []int{1, -1, 2, -99, 3}) {
		t.Fatal("離散事件被移動合併跨越", got)
	}
	for _, retained := range q.items {
		if retained != 0 {
			t.Fatal("消費後仍保留事件")
		}
	}
}

func TestQueueWakeAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New[int](2, nil)
		done := make(chan struct{})
		result := make(chan int, 1)
		go func() { value, _ := q.Pop(done); result <- value }()
		synctest.Wait()
		q.Push(42, 0)
		synctest.Wait()
		if <-result != 42 {
			t.Fatal("新增事件未喚醒消費者")
		}
		// 清空後 ready 可能仍有舊通知，不能因此假裝取得另一筆事件。
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			if _, ok := q.Pop(done); ok {
				t.Error("空佇列回傳了事件")
			}
		}()
		synctest.Wait()
		close(done)
		synctest.Wait()
		<-stopped
		q.Push(99, 0)
		if _, ok := q.Pop(done); ok || q.Len() != 1 {
			t.Fatal("取消後仍重播排隊事件")
		}
	})
}
