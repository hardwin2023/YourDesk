package clientui

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 只建立、簽署、唯讀掛載及驗證隔離的微型套件，不啟動其程式或更動安裝。
func TestLocalDMGNativeSmoke(t *testing.T) {
	if os.Getenv("YOURDESK_NATIVE_UPDATE_TESTS") != "1" {
		t.Skip("設定 YOURDESK_NATIVE_UPDATE_TESTS=1 執行隔離的原生 DMG 驗證")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, scenario := range []string{"valid-rollback", "wrong-id", "wrong-arch", "tampered", "invalid-version"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			app := filepath.Join(dir, "image-root", "YourDesk.app")
			mac := filepath.Join(app, "Contents", "MacOS")
			if err := os.MkdirAll(mac, 0700); err != nil {
				t.Fatal(err)
			}
			id, version := "com.yourdesk.desktop", "1.20.0101 build 0000"
			if scenario == "wrong-id" {
				id = "com.example.other"
			}
			if scenario == "invalid-version" {
				version = "unknown"
			}
			plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>%s</string><key>CFBundleExecutable</key><string>YourDesk</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleGetInfoString</key><string>%s</string></dict></plist>`, id, version)
			os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0600)
			arch := "arm64"
			if runtime.GOARCH == "amd64" {
				arch = "x86_64"
			}
			if scenario == "wrong-arch" {
				if arch == "arm64" {
					arch = "x86_64"
				} else {
					arch = "arm64"
				}
			}
			cmd := exec.CommandContext(ctx, "/usr/bin/clang", "-arch", arch, "-x", "c", "-", "-o", filepath.Join(mac, "YourDesk"))
			cmd.Stdin = strings.NewReader("int main(void){return 0;}\n")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("微型測試程式編譯失敗：%v %s", err, out)
			}
			binary, _ := os.ReadFile(filepath.Join(mac, "YourDesk"))
			for _, name := range []string{"yourdesk-client", "yourdesk-remote"} {
				os.WriteFile(filepath.Join(mac, name), binary, 0700)
			}
			if out, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--force", "--deep", "--sign", "-", "--timestamp=none", app).CombinedOutput(); err != nil {
				t.Fatalf("測試簽章失敗：%v %s", err, out)
			}
			if scenario == "tampered" {
				os.WriteFile(filepath.Join(mac, "yourdesk-client"), append(binary, 'x'), 0700)
			}
			archive := filepath.Join(dir, "offline.dmg")
			if out, err := exec.CommandContext(ctx, "/usr/bin/hdiutil", "create", "-srcfolder", filepath.Join(dir, "image-root"), "-volname", "YourDesk Offline Smoke", "-fs", "HFS+", "-format", "UDZO", archive).CombinedOutput(); err != nil {
				t.Fatalf("測試 DMG 建立失敗：%v %s", err, out)
			}
			before, _ := os.ReadFile(archive)
			stage := filepath.Join(dir, "stage")
			os.Mkdir(stage, 0700)
			metadata, err := inspectLocalUpdate(ctx, archive, stage)
			valid := scenario == "valid-rollback"
			if (err == nil) != valid || valid && metadata.Version != version {
				t.Fatalf("DMG 驗證不符：%+v %v", metadata, err)
			}
			after, _ := os.ReadFile(archive)
			if sha256.Sum256(before) != sha256.Sum256(after) {
				t.Fatal("唯讀驗證不應修改原始套件")
			}
			if _, err := os.Stat(filepath.Join(stage, "volume")); !os.IsNotExist(err) {
				t.Fatal("沒有卸載或清除 DMG 掛載點")
			}
		})
	}
}
