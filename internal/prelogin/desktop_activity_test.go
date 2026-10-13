package prelogin

import (
	"errors"
	"sync"
	"testing"
)

func TestDesktopActivityLifetime(t *testing.T) {
	a := newDesktopActivity(false)
	if a.allowed(1) || a.allowed(0) || a.allowed(0xffffffff) {
		t.Fatal("未啟動 APP 不得接受連線")
	}
	first, _, err := a.hold(1)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := a.hold(1)
	if err != nil {
		t.Fatal(err)
	}
	if !a.allowed(1) || a.allowed(2) {
		t.Fatal("租約只能授予該主控台工作階段")
	}
	first()
	first()
	if !a.allowed(1) {
		t.Fatal("另一份有效租約仍應保留 Host")
	}
	second()
	if a.allowed(1) {
		t.Fatal("最後一份租約退出必須回收 Host")
	}
}

func TestDesktopActivityAutostartIsIndependent(t *testing.T) {
	a := newDesktopActivity(false)
	release, _, err := a.hold(7)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	writes := 0
	save := func(bool) error { writes++; return nil }
	if err := a.setAutostart(true, save); err != nil {
		t.Fatal(err)
	}
	if !a.allowed(9) || a.allowed(0) {
		t.Fatal("登入前啟動的工作階段限制錯誤")
	}
	if err := a.setAutostart(true, save); err != nil || writes != 1 {
		t.Fatal("相同設定不應重寫")
	}
	if err := a.setAutostart(false, func(bool) error { return errors.New("磁碟失敗") }); err == nil || !a.allowed(9) {
		t.Fatal("儲存失敗不得改變執行狀態")
	}
	if err := a.setAutostart(false, save); err != nil {
		t.Fatal(err)
	}
	if !a.allowed(7) || a.allowed(9) {
		t.Fatal("停用開機連線仍須保留 APP 的租約")
	}
	release()
	if a.allowed(7) {
		t.Fatal("APP 結束後應回收")
	}
}

func TestDesktopActivityBoundedLeases(t *testing.T) {
	a := newDesktopActivity(false)
	for _, session := range []uint32{0, 0xffffffff} {
		if _, _, err := a.hold(session); err == nil {
			t.Fatal("不得授予無效工作階段")
		}
	}
	var release []func()
	for i := 0; i < 4; i++ {
		f, _, err := a.hold(1)
		if err != nil {
			t.Fatal(err)
		}
		release = append(release, f)
	}
	if _, _, err := a.hold(1); err == nil {
		t.Fatal("長連線不得占滿設定與更新管線")
	}
	var wg sync.WaitGroup
	for _, f := range release {
		wg.Go(func() { f(); f() })
	}
	wg.Wait()
	if a.allowed(1) {
		t.Fatal("並行回收失敗")
	}
	f, _, err := a.hold(2)
	if err != nil {
		t.Fatal("回收後應可重新連線", err)
	}
	f()
}

func TestDesktopActivityRevokesChangedConsole(t *testing.T) {
	a := newDesktopActivity(false)
	release, revoked, err := a.hold(10)
	if err != nil {
		t.Fatal(err)
	}
	a.retainConsole(11)
	select {
	case <-revoked:
	default:
		t.Fatal("工作階段切換未中斷舊租約")
	}
	if a.allowed(10) || a.allowed(11) {
		t.Fatal("新的主控台不能沿用舊授權")
	}
	// 回到原工作階段，須由 APP 重新握手；延遲的舊回收不能釋放新租約。
	next, _, err := a.hold(10)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if !a.allowed(10) {
		t.Fatal("舊管線回收釋放了新的租約")
	}
	next()
}
