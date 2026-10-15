package p2p

import "encoding/json"

func encodeControl(c Control) ([]byte, error)  { return json.Marshal(c) }
func decodeControl(b []byte, c *Control) error { return json.Unmarshal(b, c) }

// CanCoalesceMoves 僅允許同一視野的連續絕對移動取代前一筆。
// 按鍵、按鈕、滾輪、重設、剪貼簿與視野變更皆保留原來的順序。
func CanCoalesceMoves(previous, next Control) bool {
	if previous.Type != "move" || next.Type != "move" || previous.ViewID != next.ViewID || previous.Platform != next.Platform || previous.DisableMapping != next.DisableMapping {
		return false
	}
	if previous.Display == nil || next.Display == nil {
		return previous.Display == nil && next.Display == nil
	}
	return *previous.Display == *next.Display
}
