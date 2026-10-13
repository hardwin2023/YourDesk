package hostsession

import (
	"sync/atomic"
	"testing"
)

func TestCaptureRecoveryStartsNewEpoch(t *testing.T) {
	var current atomic.Uint64
	current.Store(7)
	queued := encodedFrames{Epoch: 7}
	restored := capturedFrame{Epoch: recoveredCaptureEpoch(&current, 7)}
	if queued.Epoch == current.Load() {
		t.Fatal("恢復後仍可傳送中斷前的影格")
	}
	if restored.Epoch != current.Load() || restored.Epoch == queued.Epoch {
		t.Fatal("恢復影格未進入新的編碼世代")
	}
}

func TestCaptureRecoveryDoesNotRelabelChangedView(t *testing.T) {
	var current atomic.Uint64
	current.Store(7)
	oldView := capturedFrame{Epoch: current.Load(), ViewID: 1}
	current.Add(1) // OS 擷取尚未返回時，另一個工作者切到新視野。
	newView := capturedFrame{Epoch: current.Load(), ViewID: 2}
	oldView.Epoch = recoveredCaptureEpoch(&current, oldView.Epoch)
	if oldView.Epoch == current.Load() || current.Load() != newView.Epoch {
		t.Fatal("恢復操作把舊視野重新標成目前影格，或作廢了新視野")
	}
}
