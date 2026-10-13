package clientui

import (
	"testing"
	"yourdesk/internal/prelogin"
)

func TestPreloginIndependentAuthorization(t *testing.T) {
	runtime := prelogin.State{Supported: true, DesktopAccessSupported: true, SASSupported: true, Installed: true}
	for _, tc := range []struct {
		name             string
		state            prelogin.State
		request          preloginChange
		needed, rejected bool
	}{
		{"已授權不重複安裝", runtime, preloginChange{Enabled: true, DesktopAccess: true}, false, false},
		{"關閉開機不移除授權", runtime, preloginChange{Enabled: false}, false, false},
		{"開啟開機仍須獨立設定", runtime, preloginChange{Enabled: true}, true, false},
		{"授權SAS不強制開機", runtime, preloginChange{Enabled: true, SecureAttention: true}, true, false},
		{"移除執行期間授權", runtime, preloginChange{Enabled: false, DesktopAccess: true}, true, false},
		{"開機服務不可直接移除", prelogin.State{Supported: true, DesktopAccessSupported: true, Installed: true, Enabled: true}, preloginChange{DesktopAccess: true}, false, true},
		{"不可混用兩種授權", runtime, preloginChange{Enabled: true, DesktopAccess: true, SecureAttention: true}, false, true},
		{"SAS關閉須使用專用開關", runtime, preloginChange{SecureAttention: true}, false, true},
		{"其他平台不能安裝", prelogin.State{Supported: true}, preloginChange{Enabled: true, DesktopAccess: true}, false, true},
		{"WinPE不能安裝", prelogin.State{}, preloginChange{Enabled: true, DesktopAccess: true}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			needed, err := preloginChangeNeeded(tc.state, tc.request)
			if needed != tc.needed || (err != nil) != tc.rejected {
				t.Fatalf("needed=%v, err=%v", needed, err)
			}
		})
	}
	if !runtime.Managed() || (prelogin.State{}).Managed() || !(prelogin.State{Enabled: true}).Managed() {
		t.Fatal("執行期間、舊版與無服務的 Host 所有權錯誤")
	}
}
