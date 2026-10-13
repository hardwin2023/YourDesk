package hostsession

import (
	"image"
	"sync/atomic"
	"yourdesk/internal/p2p"
)

// 擷取取得獨立影像；縮圖及編碼資源只由編碼階段持有。
type capturedFrame struct {
	ViewID  uint64
	Image   image.Image
	Display int
	Epoch   uint64
}

// 擷取期間若切換了視野，舊影格仍要保留舊世代，由傳送階段丟棄；
// 只有仍屬於目前視野的恢復影格才建立新世代及重置編碼參考。
func recoveredCaptureEpoch(current *atomic.Uint64, captured uint64) uint64 {
	if current.CompareAndSwap(captured, captured+1) {
		return captured + 1
	}
	return captured
}

// 一次擷取產生的 JPEG 區塊整批交付，避免跨影格交錯。
type encodedFrames struct {
	ViewID          uint64
	Frames          []p2p.Frame
	Epoch, Recovery uint64
	BitrateLimit    int // 編碼當下的傳送上限快照，避免跨工作者讀取設定。
}
