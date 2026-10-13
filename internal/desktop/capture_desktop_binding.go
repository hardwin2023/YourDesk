package desktop

import "strings"

// 只由固定的擷取執行緒使用；切換前釋放舊桌面資源，失敗時不擷取舊畫面。
type captureDesktopBinding struct {
	open    func() (uintptr, string, error)
	bind    func(uintptr) error
	release func(uintptr)
	current uintptr
	name    string
	stale   bool
}

func (b *captureDesktopBinding) ensure(reset func()) error {
	h, name, err := b.open()
	if err != nil {
		// 安全桌面可能暫時不可存取，DXGI／GDI 此時已不再有效。
		// 回到同名桌面仍須重建，不能因名稱相同沿用中斷前的資源。
		if !b.stale {
			reset()
		}
		b.stale = true
		return err
	}
	if b.current != 0 && !b.stale && strings.EqualFold(name, b.name) {
		b.release(h)
		return nil
	}
	reset()
	if err := b.bind(h); err != nil {
		b.stale = true
		b.release(h)
		return err
	}
	previous := b.current
	b.current, b.name = h, name
	b.stale = false
	if previous != 0 {
		b.release(previous)
	}
	return nil
}
