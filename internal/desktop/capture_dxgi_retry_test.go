package desktop

import (
	"testing"
	"time"
)

func TestDXGIDesktopRecoveryRetriesPromptly(t *testing.T) {
	for _, hr := range []uint32{0x887A0026, 0x887A0005, 0x887A0007, 0x887A0022, 0x887A0028, 0x80070005} {
		if delay := dxgiRetryDelay(hr); delay <= 0 || delay > time.Second {
			t.Fatalf("暫時切換狀態 0x%08X 仍需長時間等待：%v", hr, delay)
		}
	}
	for _, hr := range []uint32{0x887A0004, 0x80070057, 0x80004005} {
		if delay := dxgiRetryDelay(hr); delay < 10*time.Second {
			t.Fatalf("不支援／未知狀態 0x%08X 過度探測：%v", hr, delay)
		}
	}
}
