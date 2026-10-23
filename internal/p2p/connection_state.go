package p2p

import (
	"github.com/pion/webrtc/v4"
	"yourdesk/internal/authlog"
)

const (
	disconnectedMessage = "遠端連線已結束，請確認網路與遠端裝置後重試。"
	iceFailureMessage   = "遠端網路連線恢復逾時，請重新連線。"
	pingFailureMessage  = "遠端控制通道持續無回應，連線已結束。"
)

func (p *Peer) bindConnectionStates() {
	p.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		authlog.Event("peer-state", map[string]any{"state": state.String()})
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			p.markClosedReason(disconnectedMessage)
		}
	})
	p.pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		authlog.Event("ice-state", map[string]any{"state": state.String()})
		// Disconnected 是可恢復狀態；ICE 持續送出檢查並保留恢復寬限。
		// 只有底層確認 Failed 才結束，避免數秒的 Wi-Fi／UDP 中斷永久拆線。
		if state == webrtc.ICEConnectionStateFailed {
			p.markClosedReason(iceFailureMessage)
			_ = p.pc.Close()
		}
	})
}

func (p *Peer) markClosedReason(reason string) {
	p.mu.Lock()
	if !p.closed {
		p.disconnectReason = reason
		p.closed = true
	}
	p.mu.Unlock()
	p.doneOnce.Do(func() { close(p.done) })
}

// DisconnectReason 在 Done 關閉後保留第一個結束原因；主動 Close 不產生錯誤提示。
func (p *Peer) DisconnectReason() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.disconnectReason
}
