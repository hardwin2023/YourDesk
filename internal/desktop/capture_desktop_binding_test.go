package desktop

import (
	"errors"
	"reflect"
	"testing"
)

func TestCaptureDesktopSwitch(t *testing.T) {
	var events []string
	var next uintptr
	name := "Default"
	denied := false
	b := captureDesktopBinding{
		open: func() (uintptr, string, error) { next++; return next, name, nil },
		bind: func(uintptr) error {
			events = append(events, "bind")
			if denied {
				return errors.New("denied")
			}
			return nil
		},
		release: func(uintptr) { events = append(events, "close") },
	}
	reset := func() { events = append(events, "reset") }
	for _, step := range []struct {
		name   string
		denied bool
		want   []string
	}{
		{"Default", false, []string{"reset", "bind"}},
		{"default", false, []string{"close"}},
		{"Winlogon", false, []string{"reset", "bind", "close"}},
		{"Default", true, []string{"reset", "bind", "close"}},
		{"Default", false, []string{"reset", "bind", "close"}},
	} {
		name, denied = step.name, step.denied
		events = nil
		previous := b.current
		err := b.ensure(reset)
		if (err != nil) != denied {
			t.Fatalf("%s: %v", name, err)
		}
		if denied && (b.current != previous || b.name != "Winlogon") {
			t.Fatal("切換失敗覆蓋了有效桌面")
		}
		if !reflect.DeepEqual(events, step.want) {
			t.Fatalf("%s: %v want %v", name, events, step.want)
		}
	}
	b.open = func() (uintptr, string, error) { return 0, "", errors.New("no desktop") }
	events = nil
	if b.ensure(reset) == nil || !reflect.DeepEqual(events, []string{"reset"}) {
		t.Fatal("桌面不可用時未清除失效擷取資源，或仍嘗試切換")
	}
}

func TestCaptureDesktopReturnsToSameNameAfterDenial(t *testing.T) {
	var next, active uintptr
	var bound, closed []uintptr
	var resets int
	var openErr, bindErr error
	name := "Default"
	b := captureDesktopBinding{
		open: func() (uintptr, string, error) {
			if openErr != nil {
				return 0, "", openErr
			}
			next++
			return next, name, nil
		},
		bind: func(h uintptr) error {
			bound = append(bound, h)
			if bindErr != nil {
				return bindErr
			}
			active = h
			return nil
		},
		release: func(h uintptr) {
			if h == active {
				t.Fatal("關閉了仍綁定的桌面")
			}
			closed = append(closed, h)
		},
	}
	reset := func() { resets++ }
	if err := b.ensure(reset); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("安全桌面存取被拒絕")
	openErr = denied
	for range 100 {
		if err := b.ensure(reset); !errors.Is(err, denied) {
			t.Fatal(err)
		}
	}
	if resets != 2 || len(bound) != 1 || b.current != 1 {
		t.Fatal("拒絕期間未清除資源，或反覆重建／切換桌面", resets, bound, b.current)
	}
	openErr = nil
	if err := b.ensure(reset); err != nil {
		t.Fatal(err)
	}
	if active != 2 || resets != 3 || !reflect.DeepEqual(closed, []uintptr{1}) {
		t.Fatal("回到同名桌面仍沿用失效的擷取資源", active, resets, closed)
	}
	// 新桌面綁定失敗，再回到原桌面也必須重建。
	name, bindErr = "Winlogon", denied
	if err := b.ensure(reset); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	name, bindErr = "default", nil
	if err := b.ensure(reset); err != nil {
		t.Fatal(err)
	}
	if active != 4 || !reflect.DeepEqual(closed, []uintptr{1, 3, 2}) {
		t.Fatal("綁定失敗後未完成重新交接", active, closed)
	}
}
