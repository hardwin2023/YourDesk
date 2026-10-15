package prelogin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func waitLeaseDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("租約未及時回收")
	}
}

func TestDesktopLeaseAppAndServiceExit(t *testing.T) {
	for _, action := range []string{"APP退出", "服務退出", "主控台切換"} {
		t.Run(action, func(t *testing.T) {
			appCtx, appCancel := context.WithCancel(context.Background())
			defer appCancel()
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()
			client, server := net.Pipe()
			activity := newDesktopActivity(false)
			serverDone, clientDone := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(serverDone)
				defer server.Close()
				decoder := json.NewDecoder(server)
				var request map[string]string
				if decoder.Decode(&request) != nil || request["operation"] != "desktop-lease" {
					return
				}
				serveDesktopLease(serverCtx, server, io.MultiReader(decoder.Buffered(), server), activity, 5)
			}()
			go func() { defer close(clientDone); _ = holdDesktopLease(appCtx, client, nil) }()
			select {
			case <-activity.changed:
			case <-time.After(time.Second):
				t.Fatal("APP 未取得租約")
			}
			if !activity.allowed(5) || activity.allowed(6) {
				t.Fatal("租約工作階段錯誤")
			}
			// JSON 握手的換行不得誤判為額外操作，造成租約立即結束。
			select {
			case <-serverDone:
				t.Fatal("握手後服務不應提前釋放租約")
			case <-time.After(10 * time.Millisecond):
			}
			switch action {
			case "服務退出":
				serverCancel()
			case "主控台切換":
				activity.retainConsole(6)
			default:
				appCancel()
			}
			waitLeaseDone(t, serverDone)
			waitLeaseDone(t, clientDone)
			if activity.allowed(5) {
				t.Fatal("退出後仍接受遠端連線")
			}
		})
	}
}

func TestDesktopLeaseHandshakeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client, server := net.Pipe()
	defer server.Close()
	done := make(chan struct{})
	var result error
	go func() { defer close(done); result = holdDesktopLease(ctx, client, nil) }()
	cancel()
	waitLeaseDone(t, done)
	if result == nil {
		t.Fatal("尚未握手的租約不得成功")
	}
}

func TestDesktopLeaseRejectsAdditionalOperation(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	activity := newDesktopActivity(false)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer server.Close()
		serveDesktopLease(context.Background(), server, strings.NewReader("\n{\"operation\":\"任意命令\"}"), activity, 3)
	}()
	if err := readDesktopReply(client); err != nil {
		t.Fatal(err)
	}
	waitLeaseDone(t, done)
	if activity.allowed(3) {
		t.Fatal("額外操作後仍持有租約")
	}
}

func TestDesktopReplyRequiresAcknowledgement(t *testing.T) {
	for _, payload := range []string{"", "true", `{}`, `{"ok":false}`, strings.Repeat(" ", 4096) + `{"ok":true}`} {
		if err := readDesktopReply(strings.NewReader(payload)); err == nil {
			t.Fatalf("無效回覆被視為成功：%q", payload[:min(30, len(payload))])
		}
	}
	if err := readDesktopReply(strings.NewReader(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := readDesktopReply(strings.NewReader(`{"error":"拒絕存取"}`)); err == nil || err.Error() != "拒絕存取" {
		t.Fatal(err)
	}
	for _, payload := range []string{`{"ok":`, `{}`, `{"ok":false}`} {
		if err := readDesktopReply(strings.NewReader(payload)); err == nil || errors.Is(err, ErrDesktopAccessUnsupported) {
			t.Fatalf("不完整或錯誤回覆不可當成舊服務：%q %v", payload, err)
		}
	}
}

func TestDesktopLeaseSegmentedReply(t *testing.T) {
	for _, extra := range []string{"", "x", `{"ok":true}`} {
		t.Run("額外資料="+extra, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client, server := net.Pipe()
			defer server.Close()
			ready, whitespace, serverDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
			go func() {
				defer close(serverDone)
				var request map[string]string
				if json.NewDecoder(server).Decode(&request) != nil {
					return
				}
				// 分開交付 JSON 與後面的空白，模擬管線分段。
				if _, err := io.WriteString(server, `{"ok":true}`); err != nil {
					return
				}
				select {
				case <-ready:
				case <-ctx.Done():
					return
				}
				if _, err := io.WriteString(server, "\r\n \t\n"); err != nil {
					return
				}
				close(whitespace)
				if extra != "" {
					_, _ = io.WriteString(server, extra)
				}
			}()
			result := make(chan error, 1)
			go func() { result <- holdDesktopLease(ctx, client, func() { close(ready) }) }()
			select {
			case <-whitespace:
			case err := <-result:
				t.Fatalf("分段空白錯誤結束租約：%v", err)
			case <-time.After(time.Second):
				t.Fatal("分段空白未完成")
			}
			if extra == "" {
				select {
				case err := <-result:
					t.Fatalf("空白應保留租約：%v", err)
				case <-time.After(10 * time.Millisecond):
				}
				cancel()
			}
			select {
			case err := <-result:
				if err == nil || (extra != "" && err.Error() != "系統操作租約收到非預期資料。") {
					t.Fatalf("租約結束原因不正確：%v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("租約未回收")
			}
			waitLeaseDone(t, serverDone)
		})
	}
}

func TestDesktopRuntimeRechecksRestoredService(t *testing.T) {
	var runtime desktopRuntime
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, server := net.Pipe()
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() {
		defer server.Close()
		var request map[string]string
		if json.NewDecoder(server).Decode(&request) != nil {
			return
		}
		if request["operation"] != "desktop-lease" {
			return
		}
		// 相容上一版：能力由操作成功確認，不要求它已支援新增的能力欄位。
		if json.NewEncoder(server).Encode(streamSettingsReply{OK: true}) != nil {
			return
		}
		<-ctx.Done()
	}()
	go func() { done <- runtime.hold(ctx, client, func() { close(ready) }) }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("新版租約未完成")
	}
	if runtime.protocol.Load() != 1 {
		t.Fatal("有效租約未確認能力")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("停止服務後未回收")
	}
	if runtime.protocol.Load() != 0 {
		t.Fatal("服務停止仍保留舊能力")
	}
	// 模擬回復不支援租約的舊服務，並故意留下上一版能力快取。
	runtime.protocol.Store(1)
	client, server = net.Pipe()
	go func() {
		defer server.Close()
		var request map[string]string
		_ = json.NewDecoder(server).Decode(&request)
	}()
	called := false
	err := runtime.hold(context.Background(), client, func() { called = true })
	if !errors.Is(err, ErrDesktopAccessUnsupported) || runtime.protocol.Load() != 0 || called {
		t.Fatalf("回復後誤認服務能力：protocol=%d ready=%v err=%v", runtime.protocol.Load(), called, err)
	}
}

func TestDesktopAutostartRequiresLiveAcknowledgement(t *testing.T) {
	denied := errors.New("服務拒絕設定")
	for _, tc := range []struct {
		name    string
		enabled bool
		reply   error
		remove  bool
		want    error
	}{
		{"新版開啟不提權", true, nil, false, nil},
		{"新版關閉不移除", false, nil, false, nil},
		{"回復舊版關閉沿用明確移除", false, ErrDesktopAccessUnsupported, true, nil},
		{"舊版不能啟用執行期間模式", true, ErrDesktopAccessUnsupported, false, ErrDesktopAccessUnsupported},
		{"離線不自動移除", false, io.ErrClosedPipe, false, io.ErrClosedPipe},
		{"設定遭拒不自動提權", false, denied, false, denied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			removed := false
			err := setInstalledDesktopAutostart(tc.enabled, func(enabled bool) error {
				if enabled != tc.enabled {
					t.Fatal("模式錯誤")
				}
				return tc.reply
			}, func() error { removed = true; return nil })
			if removed != tc.remove || !errors.Is(err, tc.want) {
				t.Fatalf("removed=%v err=%v", removed, err)
			}
		})
	}
}

func TestDesktopLeaseRejectsBufferedOperation(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	go func() {
		var request map[string]string
		if json.NewDecoder(server).Decode(&request) == nil {
			_, _ = io.WriteString(server, `{"ok":true}`+"\n"+`{"operation":"額外要求"}`)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := holdDesktopLease(ctx, client, nil)
	if err == nil || err.Error() != "系統操作租約收到非預期資料。" {
		t.Fatalf("解碼器緩衝內的額外要求沒有拒絕：%v", err)
	}
}
