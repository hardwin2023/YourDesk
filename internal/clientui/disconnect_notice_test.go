package clientui

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// 真正經過子程序輸出、事件解析、程序正常結束；視窗自動關閉後仍可看到原因。
func TestViewerDisconnectNoticeSmoke(t *testing.T) {
	if message, ok := os.LookupEnv("YOURDESK_DISCONNECT_SMOKE"); ok {
		body, _ := json.Marshal(map[string]string{"event": "disconnected", "message": message})
		fmt.Println("YOURDESK_UI_EVENT " + string(body))
		os.Exit(0)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"遠端網路連線恢復逾時，請重新連線。", "遠端控制通道持續無回應，連線已結束。", ""} {
		t.Setenv("YOURDESK_DISCONNECT_SMOKE", message)
		s := &server{children: map[string]*process{}, viewerClosed: make(chan struct{}, 1), preferences: Preferences{CloseWindowOnDisconnect: true}}
		s.mu.Lock()
		err := s.start("viewer", "smoke", binary, []string{"-test.run=^TestViewerDisconnectNoticeSmoke$"})
		p := s.children["viewer:smoke"]
		s.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
			_ = p.cmd.Process.Kill()
			t.Fatal("子程序斷線處理逾時")
		}
		want := message
		s.mu.Lock()
		notice := s.notice
		s.mu.Unlock()
		if notice != want {
			t.Fatalf("視窗關閉後提示為 %q，預期 %q", notice, want)
		}
	}
}
