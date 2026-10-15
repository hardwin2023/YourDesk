package clipboard

import (
	"context"
	"fmt"
	"yourdesk/internal/p2p"
)

// PasteAgentText 沿用既有剪貼簿傳输並等待完成；會取代本機與遠端剪貼簿文字。
func (s *Sync) PasteAgentText(ctx context.Context, text string) error {
	if len(text) == 0 || len(text) > 16384 || !validText([]byte(text)) {
		return fmt.Errorf("文字必須是有效 UTF-8，且不超過 16 KB")
	}
	s.nativeMu.Lock()
	err := writeContent(content{Kind: "text", Data: []byte(text)})
	s.nativeMu.Unlock()
	if err != nil {
		return err
	}
	if err = s.transferForPaste(ctx); err != nil {
		return err
	}
	// 指定 Windows 按鍵語意，由 Host 的既有跨平台映射轉換貼上組合鍵。
	controls := []p2p.Control{{Type: "key", Key: "control", Down: true}, {Type: "key", Key: "v", Down: true}, {Type: "key", Key: "v", Down: false}, {Type: "key", Key: "control", Down: false}}
	for i := range controls {
		controls[i].Platform = "windows"
	}
	return s.enqueueInputBatch(ctx, controls)
}

// 批次按鍵沿用同一有序工作者，傳輸或等待結果均不持有 UI 輸入鎖。
func (s *Sync) enqueueInputBatch(ctx context.Context, controls []p2p.Control) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(controls) == 0 {
		return nil
	}
	s.mu.Lock()
	if s.peer == nil || s.ctx == nil {
		s.mu.Unlock()
		return fmt.Errorf("操作連線尚未就緒")
	}
	if s.inputEpoch == nil {
		s.inputEpoch = make(chan struct{})
	}
	epoch, sessionDone := s.inputEpoch, s.ctx.Done()
	result := make(chan error, 1)
	accepted := s.keys.Push(keyJob{batch: append([]p2p.Control(nil), controls...), result: result, context: ctx, generation: s.generation}, 0)
	s.mu.Unlock()
	if !accepted {
		return fmt.Errorf("輸入佇列已滿")
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-sessionDone:
		return context.Canceled
	case <-epoch:
		select {
		case err := <-result:
			return err
		default:
			return context.Canceled
		}
	}
}
