package input

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// 模擬 Default → 螢幕保護桌面 → Default，不呼叫真實鍵鼠 API。
func TestInputDesktopChangesAndRecovers(t *testing.T) {
	var next, active uintptr
	var delivered, closed []uintptr
	denied := errors.New("桌面暫時不可存取")
	var openErr, bindErr error
	b := desktopBinding{
		open: func() (inputDesktop, error) {
			next++
			return inputDesktop{handle: next, name: fmt.Sprint(next)}, openErr
		},
		bind: func(h uintptr) error {
			if bindErr != nil {
				return bindErr
			}
			active = h
			return nil
		},
		close: func(h uintptr) {
			if h == active {
				t.Fatal("關閉了仍綁定的桌面")
			}
			closed = append(closed, h)
		},
	}
	inject := func() error { delivered = append(delivered, active); return nil }
	for range 2 {
		if err := b.run(inject); err != nil {
			t.Fatal(err)
		}
	}
	openErr = denied
	if err := b.run(inject); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	openErr, bindErr = nil, denied
	if err := b.run(inject); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(delivered, []uintptr{1, 2}) {
		t.Fatal("拒絕期間仍注入輸入", delivered)
	}
	bindErr = nil
	if err := b.run(inject); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(delivered, []uintptr{1, 2, 5}) {
		t.Fatal("恢復後重播了失敗事件", delivered)
	}
	if !reflect.DeepEqual(closed, []uintptr{1, 4, 2}) || b.current.handle != 5 {
		t.Fatal("桌面資源未正確交接", closed, b.current)
	}
	failed := errors.New("輸入未被接受")
	if err := b.run(func() error { return failed }); !errors.Is(err, failed) {
		t.Fatal(err)
	}
}

func TestInputDesktopDoesNotRebindSameDesktop(t *testing.T) {
	var next uintptr
	var bound, closed []uintptr
	b := desktopBinding{
		open:  func() (inputDesktop, error) { next++; return inputDesktop{handle: next, name: "Default"}, nil },
		bind:  func(h uintptr) error { bound = append(bound, h); return nil },
		close: func(h uintptr) { closed = append(closed, h) },
	}
	for range 100 {
		if err := b.run(func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(bound, []uintptr{1}) || len(closed) != 99 || closed[0] != 2 || closed[98] != 100 {
		t.Fatal("同桌面重複綁定或未釋放查詢 handle", bound, closed)
	}
}

func TestInputDesktopRebindsAfterAccessAndInjectionFailure(t *testing.T) {
	var next, active uintptr
	var bound, delivered []uintptr
	var openErr, bindErr error
	name := "Default"
	b := desktopBinding{
		open: func() (inputDesktop, error) {
			if openErr != nil {
				return inputDesktop{}, openErr
			}
			next++
			return inputDesktop{handle: next, name: name}, nil
		},
		bind: func(h uintptr) error {
			if bindErr != nil {
				return bindErr
			}
			active = h
			bound = append(bound, h)
			return nil
		},
		close: func(h uintptr) {
			if h == active {
				t.Fatal("關閉了仍綁定的輸入桌面")
			}
		},
	}
	inject := func() error { delivered = append(delivered, active); return nil }
	if err := b.run(inject); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("安全桌面不允許輸入")
	openErr = denied
	for range 100 {
		if err := b.run(inject); !errors.Is(err, denied) {
			t.Fatal(err)
		}
	}
	openErr = nil
	if err := b.run(inject); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bound, []uintptr{1, 2}) || !reflect.DeepEqual(delivered, []uintptr{1, 2}) {
		t.Fatal("未重新綁定同名桌面，或重播了拒絕期間的輸入", bound, delivered)
	}
	name, bindErr = "Winlogon", denied
	if err := b.run(inject); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	name, bindErr = "Default", nil
	if err := b.run(inject); err != nil {
		t.Fatal(err)
	}
	// SendInput 失敗可能只被注入層發現，下個新事件同樣要重新綁定。
	if err := b.run(func() error { return denied }); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	if err := b.run(inject); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bound, []uintptr{1, 2, 4, 6}) || !reflect.DeepEqual(delivered, []uintptr{1, 2, 4, 6}) {
		t.Fatal("綁定／注入錯誤後仍使用舊桌面，或重新注入了失敗事件", bound, delivered)
	}
}
