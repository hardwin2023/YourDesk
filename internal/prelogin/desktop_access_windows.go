package prelogin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	winio "github.com/tailscale/go-winio"
)

const desktopControlMarker = "desktop-control.v1"
const desktopAutostartDisabled = "autostart.disabled"
const desktopMarkerACL = "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;GR;;;AU)"

var desktopRuntimeControl desktopRuntime

func readDesktopMarker(root, name string) ([]byte, error) {
	path := filepath.Join(root, name)
	if err := noReparsePath(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8 {
		return nil, errors.New("系統服務模式標記無效。")
	}
	return io.ReadAll(io.LimitReader(f, 9))
}

func desktopAutostart(root string) (bool, error) {
	data, err := readDesktopMarker(root, desktopAutostartDisabled)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil // 舊版服務保持原來的開機連線設定。
	}
	if err != nil {
		return false, err
	}
	if len(data) != 0 {
		return false, errors.New("系統服務模式標記無效。")
	}
	return false, nil
}

func saveDesktopAutostart(root string, enabled bool) error {
	if _, err := desktopAutostart(root); err != nil {
		return err
	}
	path := filepath.Join(root, desktopAutostartDisabled)
	if enabled {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	err := secureWrite(path, nil, desktopMarkerACL)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	return err
}

func markDesktopControl(root string) error {
	data, err := readDesktopMarker(root, desktopControlMarker)
	if err == nil {
		if string(data) != "1" {
			return errors.New("系統服務能力標記無效。")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return secureWrite(filepath.Join(root, desktopControlMarker), []byte("1"), desktopMarkerACL)
}

func installDesktopAccess(path string) error { return installService(path, false) }

func RemoveService(ctx context.Context) error {
	state := Status()
	if !state.DesktopAccessSupported {
		return errors.New("此平台不支援系統操作授權。")
	}
	// 呼叫端由使用者明確確認移除；即使開機設定或管線損壞也可交由 UAC 清除。
	return elevateWindows(ctx, "remove", "")
}

func ConfigureDesktopAccess(ctx context.Context, enabled bool, c Config) error {
	state := Status()
	if !state.DesktopAccessSupported {
		return errors.New("此平台不支援系統操作授權。")
	}
	if enabled {
		if state.Managed() {
			return nil
		}
		return configureWindowsMode(ctx, true, c, "install-runtime")
	}
	if state.Enabled {
		return errors.New("請先關閉登入前啟動，再移除系統操作授權。")
	}
	if !state.Installed {
		return nil
	}
	return configureWindowsMode(ctx, false, c, "")
}

func dialDesktopControl(ctx context.Context) (net.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(dialCtx, controlPipe)
	if err != nil {
		return nil, errors.New("系統操作服務尚未就緒，請稍後重試。")
	}
	if err = verifyControlServer(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// APP 存活時持有管線；APP 結束或崩潰，SYSTEM Host 即可自動回收。
func HoldDesktopAccess(ctx context.Context, ready func()) error {
	conn, err := dialDesktopControl(ctx)
	if err != nil {
		desktopRuntimeControl.protocol.Store(0)
		return err
	}
	return desktopRuntimeControl.hold(ctx, conn, ready)
}

func requestDesktopAutostart(ctx context.Context, enabled bool) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := dialDesktopControl(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	if err = json.NewEncoder(conn).Encode(map[string]any{"operation": "autostart-settings", "enabled": enabled}); err != nil {
		return err
	}
	return readDesktopReply(conn)
}
