package prelogin

import (
	"errors"
	"sync"
)

// desktopActivity 只授予目前 OS 工作階段的租約，不接受使用者提供的 PID 或 session。
type desktopActivity struct {
	mu        sync.Mutex
	autostart bool
	leases    map[uint32]*desktopLeaseGroup
	count     int
	changed   chan struct{}
}

type desktopLeaseGroup struct {
	count int
	done  chan struct{}
}

func newDesktopActivity(autostart bool) *desktopActivity {
	return &desktopActivity{autostart: autostart, leases: make(map[uint32]*desktopLeaseGroup), changed: make(chan struct{}, 1)}
}

func (a *desktopActivity) notify() {
	select {
	case a.changed <- struct{}{}:
	default:
	}
}

func (a *desktopActivity) hold(session uint32) (func(), <-chan struct{}, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// 控制管線共八個席位，長連線最多四個，保留設定與更新操作的空間。
	if session == 0 || session == 0xffffffff || a.count >= 4 {
		return nil, nil, errors.New("系統操作租約無效或已達上限。")
	}
	group := a.leases[session]
	if group == nil {
		group = &desktopLeaseGroup{done: make(chan struct{})}
		a.leases[session] = group
	}
	group.count++
	a.count++
	a.notify()
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.leases[session] != group {
				return // 工作階段切換已回收，不能扣除新取得的租約。
			}
			group.count--
			if group.count == 0 {
				delete(a.leases, session)
			}
			a.count--
			a.notify()
		})
	}, group.done, nil
}

// 主控台切換時立即撤銷舊工作階段，不讓背景 APP 的管線占住租約。
func (a *desktopActivity) retainConsole(current uint32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for session, group := range a.leases {
		if session != current {
			close(group.done)
			a.count -= group.count
			delete(a.leases, session)
			a.notify()
		}
	}
}

func (a *desktopActivity) allowed(session uint32) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return session != 0 && session != 0xffffffff && (a.autostart || a.leases[session] != nil)
}

func (a *desktopActivity) setAutostart(enabled bool, save func(bool) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if enabled == a.autostart {
		return nil
	}
	if err := save(enabled); err != nil {
		return err
	}
	a.autostart = enabled
	a.notify()
	return nil
}
