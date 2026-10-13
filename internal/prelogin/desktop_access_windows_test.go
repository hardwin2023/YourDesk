package prelogin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopAutostartMarkers(t *testing.T) {
	root := t.TempDir()
	if on, err := desktopAutostart(root); err != nil || !on {
		t.Fatal("舊服務應保留登入前啟動", on, err)
	}
	if err := saveDesktopAutostart(root, false); err != nil {
		t.Fatal(err)
	}
	if on, err := desktopAutostart(root); err != nil || on {
		t.Fatal("執行期間模式未保存", on, err)
	}
	if err := saveDesktopAutostart(root, false); err != nil {
		t.Fatal("重複設定應成功", err)
	}
	if err := saveDesktopAutostart(root, true); err != nil {
		t.Fatal(err)
	}
	if on, err := desktopAutostart(root); err != nil || !on {
		t.Fatal("開機設定未保存", on, err)
	}
	path := filepath.Join(root, desktopAutostartDisabled)
	if err := os.WriteFile(path, []byte("損壞"), 0600); err != nil {
		t.Fatal(err)
	}
	if on, err := desktopAutostart(root); err == nil || on {
		t.Fatal("無效標記不能啟用連線", on, err)
	}
	if err := saveDesktopAutostart(root, true); err == nil {
		t.Fatal("不能靜默覆寫無效模式")
	}
}
