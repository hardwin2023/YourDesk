package prelogin

import (
	"context"
	"encoding/json"
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
}
