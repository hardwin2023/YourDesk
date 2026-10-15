package clientui

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"yourdesk/internal/prelogin"
)

func TestPresenceCapabilitiesSmoke(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "capabilities", true: "legacy"}[legacy], func(t *testing.T) {
			calls := 0
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var in struct {
					Details bool `json:"details"`
				}
				json.NewDecoder(r.Body).Decode(&in)
				w.Header().Set("Content-Type", "application/json")
				if legacy && in.Details {
					http.Error(w, "unknown field", 400)
					return
				}
				if legacy {
					w.Write([]byte(`{"room":true}`))
				} else {
					w.Write([]byte(`{"room":{"online":true,"capabilities":{"schema":1,"desktop":false,"terminal":true,"clipboard":false}}}`))
				}
			}))
			defer srv.Close()
			cert := filepath.Join(t.TempDir(), "ca.pem")
			if e := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600); e != nil {
				t.Fatal(e)
			}
			t.Setenv("YOURDESK_TLS_CA", cert)
			s := &server{library: Library{Sites: []Site{{ID: "site", Room: "room", Signal: "wss" + strings.TrimPrefix(srv.URL, "https") + "/ws"}}}}
			rec := httptest.NewRecorder()
			s.servePresence(rec, httptest.NewRequest("GET", "/api/presence", nil))
			var result map[string]struct {
				Online       *bool `json:"online"`
				Capabilities *struct {
					Desktop  bool
					Terminal bool
				} `json:"capabilities"`
			}
			if e := json.Unmarshal(rec.Body.Bytes(), &result); e != nil {
				t.Fatal(e)
			}
			got := result["site"]
			if got.Online == nil || !*got.Online {
				t.Fatalf("在線狀態錯誤: %s", rec.Body.String())
			}
			if legacy {
				if got.Capabilities != nil || calls != 2 {
					t.Fatal("舊版降級錯誤")
				}
			} else {
				if got.Capabilities == nil || got.Capabilities.Desktop || !got.Capabilities.Terminal {
					t.Fatal("能力映射錯誤")
				}
			}
		})
	}
}

func TestLocalPresenceUsesEffectiveHostIdentity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		service prelogin.State
		room    string
	}{
		{"一般 Host", prelogin.State{}, "user"},
		{"登入前服務", prelogin.State{Enabled: true, Room: "service"}, "service"},
		{"執行期間授權", prelogin.State{Installed: true, Room: "service"}, "service"},
		{"服務缺少公開 ID", prelogin.State{Installed: true}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := localPresenceTarget("user", "wss://signal.test/ws", tc.service)
			if target.ID != "host" || target.Room != tc.room || target.Signal != "wss://signal.test/ws" {
				t.Fatal("查詢了錯誤 Host，或把使用者 ID 當成服務 ID", target)
			}
		})
	}
}

func TestLocalPresenceDoesNotInferAvailabilityFromServiceInstallation(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		known, online  bool
	}{
		{"離線", `{"service":{"online":false}}`, true, false},
		{"在線含版本", `{"service":{"online":true,"capabilities":{"schema":1,"version":"1.26.1003 build 2028"}}}`, true, true},
		{"舊版在線", `{"service":true}`, true, true},
		{"缺少資料", `{}`, false, false},
		{"無效資料", `{"service":"invalid"}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct{ Rooms []string }
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Rooms) != 1 || request.Rooms[0] != "service" {
					t.Error("本機查詢帶入其他 ID，或讀取配對密碼")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tc.response))
			}))
			defer srv.Close()
			cert := filepath.Join(t.TempDir(), "ca.pem")
			if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("YOURDESK_TLS_CA", cert)
			target := localPresenceTarget("user", "wss"+strings.TrimPrefix(srv.URL, "https")+"/ws", prelogin.State{Installed: true, Enabled: true, Room: "service"})
			got := queryPresence(context.Background(), []Site{target})[target.ID]
			if (got.Online != nil) != tc.known || (got.Online != nil && *got.Online != tc.online) {
				t.Fatalf("安裝狀態蓋過實際在線證據：%+v", got)
			}
			if tc.name == "在線含版本" && (got.Capabilities == nil || got.Capabilities.Version != "1.26.1003 build 2028") {
				t.Fatal("未保留真正 Host 的版本")
			}
		})
	}
}
