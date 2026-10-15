package clientui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
	"yourdesk/internal/prelogin"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
)

type presence struct {
	Online       *bool                       `json:"online"`
	Capabilities *signaling.HostCapabilities `json:"capabilities,omitempty"`
}

// 安裝模式不代表 Host 已註冊；本機與站台使用同一個配對伺服器在線查詢。
func (s *server) serveLocalPresence(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	service := s.preloginState()
	target := localPresenceTarget(s.info["room"], s.options.Signal, service)
	s.mu.Unlock()
	result := struct {
		Room string `json:"room"`
		presence
	}{Room: target.Room}
	if target.Room != "" {
		result.presence = queryPresence(r.Context(), []Site{target})[target.ID]
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, result)
}

func localPresenceTarget(room, signal string, service prelogin.State) Site {
	if service.Managed() {
		room = service.Room // 缺少服務 ID 時不能拿普通使用者 ID 假裝服務可用。
	}
	return Site{ID: "host", Room: room, Signal: signal}
}

func (s *server) servePresence(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	sites := append([]Site(nil), s.library.Sites...)
	s.mu.Unlock()
	states := queryPresence(r.Context(), sites)
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, states)
}

func queryPresence(ctx context.Context, sites []Site) map[string]presence {
	states := make(map[string]presence, len(sites))
	groups := make(map[string][]Site)
	for _, site := range sites {
		states[site.ID] = presence{}
		if _, direct := signaling.DirectAddress(site.Room); direct {
			continue
		}
		groups[site.Signal] = append(groups[site.Signal], site)
	}
	var mu sync.Mutex
	var pending sync.WaitGroup
	slots := make(chan struct{}, 4)
	client, err := security.TLSHTTPClient(3 * time.Second)
	if err != nil {
		return states
	}
	// 本次查詢使用獨立連線池，所有工作結束後釋放閒置連線。
	defer client.CloseIdleConnections()
	for signal, group := range groups {
		pending.Add(1)
		go func(signal string, group []Site) {
			defer pending.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			for start := 0; start < len(group); start += 256 {
				batch := group[start:min(start+256, len(group))]
				rooms := make([]string, 0, len(batch))
				for _, site := range batch {
					rooms = append(rooms, site.Room)
				}
				body, _ := json.Marshal(map[string]any{"rooms": rooms, "details": true})
				response, err := security.SignalHTTPRequest(ctx, client, signal, "POST", "/presence", body)
				if err != nil {
					// 舊 Server 可能拒絕 details；共用 HTTP 層已關閉失敗回應。
					body, _ = json.Marshal(map[string]any{"rooms": rooms})
					response, err = security.SignalHTTPRequest(ctx, client, signal, "POST", "/presence", body)
					if err != nil {
						return
					}
				}
				var result map[string]json.RawMessage
				if response.StatusCode == 200 {
					err = json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&result)
				}
				response.Body.Close()
				if response.StatusCode != 200 || err != nil {
					return
				}
				mu.Lock()
				for _, site := range batch {
					if value, ok := result[site.Room]; ok {
						var item presence
						if json.Unmarshal(value, &item) != nil {
							var online bool
							if json.Unmarshal(value, &online) != nil {
								continue
							}
							item.Online = &online
						}
						if item.Capabilities != nil && item.Capabilities.Schema != 1 {
							item.Capabilities = nil
						}
						states[site.ID] = item
					}
				}
				mu.Unlock()
			}
		}(signal, group)
	}
	pending.Wait()
	return states
}
