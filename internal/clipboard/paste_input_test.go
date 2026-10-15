package clipboard

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"yourdesk/internal/p2p"
)

func TestAgentPasteBatchDoesNotHoldUIInputLock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s := &Sync{ctx: ctx, peer: &p2p.Peer{}, keys: newInputQueue()}
		entered, release := make(chan struct{}), make(chan struct{})
		var sent []p2p.Control
		go s.keyboardWithSender(ctx, func(c p2p.Control) error {
			if len(sent) == 0 {
				close(entered)
				<-release
			}
			sent = append(sent, c)
			return nil
		})
		result := make(chan error, 1)
		go func() {
			result <- s.enqueueInputBatch(ctx, []p2p.Control{{Type: "key", Key: "control", Down: true}, {Type: "key", Key: "v", Down: true}, {Type: "key", Key: "v", Down: false}, {Type: "key", Key: "control", Down: false}})
		}()
		<-entered
		s.CancelKeys("失焦")
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		close(release)
		synctest.Wait()
		if len(sent) != 3 || !sent[0].Down || sent[1].Down || sent[1].Key != "control" || sent[2].Type != "raw-key-reset" {
			t.Fatal("批次取消仍送出貼上鍵，或未釋放已送出的 Control", sent)
		}
		cancel()
		synctest.Wait()
	})
}

func TestAgentPasteWaitsForActualOrderedTransmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s := &Sync{ctx: ctx, peer: &p2p.Peer{}, keys: newInputQueue()}
		last, release := make(chan struct{}), make(chan struct{})
		var sent []p2p.Control
		go s.keyboardWithSender(ctx, func(c p2p.Control) error {
			if c.Key == "control" && !c.Down {
				close(last)
				<-release
			}
			sent = append(sent, c)
			return nil
		})
		result := make(chan error, 1)
		controls := []p2p.Control{{Type: "key", Key: "control", Down: true}, {Type: "key", Key: "v", Down: true}, {Type: "key", Key: "v", Down: false}, {Type: "key", Key: "control", Down: false}}
		go func() { result <- s.enqueueInputBatch(ctx, controls) }()
		<-last
		select {
		case <-result:
			t.Fatal("只排入佇列就回報貼上已完成")
		default:
		}
		close(release)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if len(sent) != 4 || len(s.held) != 0 {
			t.Fatal("批次按鍵未依序完整送出或留下卡鍵", sent)
		}
		cancel()
		synctest.Wait()
	})
}
