package clientui

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func localManager(t *testing.T) *updateManager {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	u := newUpdateManager(ctx, t.TempDir())
	u.forceUpdate = true
	t.Cleanup(func() { cancel(); u.downloads.Wait() })
	return u
}

func selectLocalFixture(t *testing.T, u *updateManager) *localUpdate {
	t.Helper()
	if _, err := u.beginLocalUpdateForSystem("使用者重新命名.ZIP", 7, "windows"); err != nil {
		t.Fatal(err)
	}
	return u.local
}

func uploadLocalFixture(t *testing.T, u *updateManager, version string) *localUpdate {
	t.Helper()
	job := selectLocalFixture(t, u)
	err := u.uploadLocalUpdate(context.Background(), job.status.ID, io.NopCloser(strings.NewReader("package")), func(ctx context.Context, archive, dir string) (localPackage, error) {
		data, err := os.ReadFile(archive)
		if err != nil || string(data) != "package" || filepath.Dir(archive) != dir {
			t.Fatalf("未使用私有暫存副本：%q, %v", data, err)
		}
		return localPackage{Version: version, RequiresAuthorization: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestLocalUpdateSelection(t *testing.T) {
	for _, tc := range []struct {
		system, name string
		size         int64
		want         bool
	}{
		{"windows", "任意名稱.ZIP", 1, true}, {"darwin", "任意名稱.DMG", 1, true},
		{"windows", "file.dmg", 1, false}, {"darwin", "file.zip", 1, false},
		{"windows", "C:\\file.zip", 1, false}, {"windows", "../file.zip", 1, false},
		{"windows", "file.zip", 0, false}, {"windows", "file.zip", maxUpdateSize + 1, false},
		{"linux", "file.zip", 1, false},
	} {
		t.Run(tc.system+tc.name, func(t *testing.T) {
			u := localManager(t)
			_, err := u.beginLocalUpdateForSystem(tc.name, tc.size, tc.system)
			if (err == nil) != tc.want {
				t.Fatalf("選取結果不符：%v", err)
			}
		})
	}
}

func TestLocalUpdateVersionsAndExplicitInstall(t *testing.T) {
	// 本地套件完全不依賴 GitHub；同版本與回退均可，由使用者確認才安裝。
	for _, version := range []string{"1.99.1231 build 2359", currentVersion(), "1.20.0101 build 0000"} {
		t.Run(version, func(t *testing.T) {
			u := localManager(t)
			u.state = updateStatus{Available: true, Version: "old OTA", Message: "preserve"}
			job := uploadLocalFixture(t, u, version)
			status := u.snapshot()
			if status.Local.Phase != "ready" || status.Local.Version != version || !status.Local.RequiresAuthorization || status.InstallAt != 0 || status.Opening {
				t.Fatalf("確認前不可倒數或關閉：%+v", status)
			}
			if _, err := u.beginLocalUpdateForSystem("other.zip", 7, "windows"); err == nil {
				t.Fatal("不能同時開始另一個本地更新")
			}
			if err := u.startDownload(nil); err == nil {
				t.Fatal("不能同時開始 OTA")
			}
			if status = u.check(context.Background(), false); status.Message != "preserve" || !status.CheckedAt.IsZero() {
				t.Fatal("本地更新途中不應執行背景 GitHub 檢查")
			}
			if err := u.installLocalUpdate("stale", nil, nil); err == nil {
				t.Fatal("不能使用過期的工作 ID")
			}
			var prepared, quit atomic.Int32
			if err := u.installLocalUpdate(job.status.ID, func(ctx context.Context, archive string) error {
				if archive != job.archive {
					t.Error("沒有沿用已驗證的副本")
				}
				prepared.Add(1)
				return nil
			}, func() { quit.Add(1) }); err != nil {
				t.Fatal(err)
			}
			u.downloads.Wait()
			status = u.snapshot()
			if prepared.Load() != 1 || quit.Load() != 1 || !status.ShowNotes || status.NotesVersion != version || status.Local.Phase != "installing" {
				t.Fatalf("就緒後沒有正確交接：%+v", status)
			}
			if _, err := os.Stat(job.dir); !os.IsNotExist(err) {
				t.Fatal("交接後未清除上傳暫存")
			}
		})
	}
}

func TestLocalUpdateFailureDoesNotQuit(t *testing.T) {
	for _, failure := range []string{"tampered", "helper"} {
		t.Run(failure, func(t *testing.T) {
			u := localManager(t)
			u.state.Message = "preserve OTA"
			job := uploadLocalFixture(t, u, "1.20.0101 build 0000")
			if failure == "tampered" {
				os.WriteFile(job.archive, []byte("changed"), 0600)
			}
			var prepared, quit atomic.Int32
			if err := u.installLocalUpdate(job.status.ID, func(context.Context, string) error {
				prepared.Add(1)
				return errors.New("helper fixture failure")
			}, func() { quit.Add(1) }); err != nil {
				t.Fatal(err)
			}
			u.downloads.Wait()
			status := u.snapshot()
			if quit.Load() != 0 || status.ShowNotes || status.Local.Phase != "failed" || status.Local.Error == "" || status.Message != "preserve OTA" {
				t.Fatalf("失敗影響執行中 APP／OTA 狀態：%+v", status)
			}
			if failure == "tampered" && prepared.Load() != 0 {
				t.Fatal("已變更的套件不可交給更新器")
			}
		})
	}
}

func TestLocalUpdateCancelDuringUpload(t *testing.T) {
	u := localManager(t)
	job := selectLocalFixture(t, u)
	reader, writer := io.Pipe()
	defer writer.Close()
	done := make(chan error, 1)
	go func() {
		done <- u.uploadLocalUpdate(context.Background(), job.status.ID, reader, func(context.Context, string, string) (localPackage, error) {
			t.Error("取消的上傳不可進入驗證")
			return localPackage{}, nil
		})
	}()
	if _, err := writer.Write([]byte("pa")); err != nil {
		t.Fatal(err)
	}
	if err := u.cancelLocalUpdate(job.status.ID); err != nil {
		t.Fatal(err)
	}
	// 舊工作收尾期間允許新選取，但舊回覆不能覆蓋新工作的狀態。
	newJob := selectLocalFixture(t, u)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("取消的上傳不應回報成功")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("取消沒有中斷檔案讀取")
	}
	job.workers.Wait()
	if status := u.snapshot(); status.Local.ID != newJob.status.ID || status.Local.Phase != "selected" {
		t.Fatalf("舊回覆影響新工作：%+v", status)
	}
}

func TestLocalUpdateValidationAndIdleCleanup(t *testing.T) {
	for _, scenario := range []string{"short", "long", "invalid", "cancel-ready", "shutdown-selected"} {
		t.Run(scenario, func(t *testing.T) {
			u := localManager(t)
			var job *localUpdate
			if scenario == "cancel-ready" {
				job = uploadLocalFixture(t, u, "1.20.0101 build 0000")
			} else {
				job = selectLocalFixture(t, u)
			}
			if scenario != "shutdown-selected" && scenario != "cancel-ready" {
				content := "package"
				if scenario == "short" {
					content = "small"
				} else if scenario == "long" {
					content = "oversized"
				}
				err := u.uploadLocalUpdate(context.Background(), job.status.ID, io.NopCloser(strings.NewReader(content)), func(context.Context, string, string) (localPackage, error) {
					return localPackage{Version: "1.20.0101 build 0000"}, errors.New("invalid fixture")
				})
				if err == nil {
					t.Fatal("無效／不完整套件應驗證失敗")
				}
			}
			if scenario == "shutdown-selected" {
				job.cancel()
			} else {
				if err := u.cancelLocalUpdate(job.status.ID); err != nil {
					t.Fatal(err)
				}
			}
			u.downloads.Wait()
			if _, err := os.Stat(job.dir); !os.IsNotExist(err) || scenario != "shutdown-selected" && u.snapshot().Local != nil {
				t.Fatal("沒有清除取消的暫存工作")
			}
		})
	}
}

func TestLocalUpdateAPIAuthenticationAndState(t *testing.T) {
	u := localManager(t)
	s := &server{updater: u, token: "secret", origin: "http://127.0.0.1"}
	for _, tc := range []struct{ token, origin string }{{"wrong", ""}, {"secret", "https://other.test"}} {
		r := httptest.NewRequest("POST", "/api/updates/local", strings.NewReader(`{"name":"update.zip","size":7}`))
		r.Header.Set("X-YourDesk-Token", tc.token)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		s.api(w, r)
		if w.Code != http.StatusForbidden || u.snapshot().Local != nil {
			t.Fatal("未授權請求建立了本地更新工作")
		}
	}
	job := uploadLocalFixture(t, u, "1.20.0101 build 0000")
	u.forceUpdate = false
	u.mu.Lock()
	u.saveLocked()
	u.mu.Unlock()
	data, err := os.ReadFile(u.path)
	if err != nil || strings.Contains(string(data), job.status.ID) || strings.Contains(string(data), job.archive) {
		t.Fatalf("暫存工作不可寫進持久化更新狀態：%s, %v", data, err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/updates/local/cancel", strings.NewReader(fmt.Sprintf(`{"id":%q}`, job.status.ID)))
	r.Header.Set("X-YourDesk-Token", "secret")
	s.api(w, r)
	var status updateStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status.Local != nil {
		t.Fatalf("取消 API 回覆不符：%d %s", w.Code, w.Body.String())
	}
}

func TestLocalUpdateCancelUnblocksHTTPUpload(t *testing.T) {
	u := localManager(t)
	ext := localUpdateExtension(runtime.GOOS)
	if ext == "" {
		t.Skip("此平台無本地更新")
	}
	status, err := u.beginLocalUpdate("test"+ext, 7)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{updater: u, token: "secret"}
	httpServer := httptest.NewServer(http.HandlerFunc(s.api))
	defer httpServer.Close()
	conn, err := net.Dial("tcp", strings.TrimPrefix(httpServer.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// 不送完 Content-Length，也不主動斷線；取消操作必須能解除伺服器的 Read。
	fmt.Fprintf(conn, "PUT /api/updates/local?id=%s HTTP/1.1\r\nHost: localhost\r\nX-YourDesk-Token: secret\r\nContent-Length: 7\r\n\r\npa", status.Local.ID)
	deadline := time.Now().Add(3 * time.Second)
	for u.snapshot().Local.Phase != "uploading" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if u.snapshot().Local.Phase != "uploading" {
		t.Fatal("HTTP 上傳未啟動")
	}
	if err := u.cancelLocalUpdate(status.Local.ID); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("取消後仍卡在 HTTP 讀取：%v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatalf("取消的請求不應成功：%d", response.StatusCode)
	}
	u.downloads.Wait()
}

func TestLocalPortablePackageInspection(t *testing.T) {
	for _, scenario := range []string{"valid", "old", "wrong-arch", "no-version", "service", "tampered"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "使用者重新命名.zip")
			file, _ := os.Create(archive)
			z := zip.NewWriter(file)
			peFile := make([]byte, 152)
			copy(peFile, "MZ")
			binary.LittleEndian.PutUint32(peFile[0x3c:], 128)
			copy(peFile[128:], "PE\x00\x00")
			machine := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
			if scenario == "wrong-arch" {
				machine = pe.IMAGE_FILE_MACHINE_ARM64
			}
			binary.LittleEndian.PutUint16(peFile[132:], machine)
			files := map[string][]byte{}
			for _, name := range []string{"YourDesk.exe", "yourdesk-client.exe", "yourdesk-remote.exe", "avcodec-62.dll", "avutil-60.dll", "swscale-9.dll"} {
				files[name] = peFile
			}
			version := "1.99.1231 build 2359"
			if scenario == "old" {
				version = "1.20.0101 build 0000"
			}
			if scenario != "no-version" {
				files["README.txt"] = []byte("\ufeffYourDesk " + version + "\r\n\r\ninstructions")
			}
			var sums strings.Builder
			prefix := "YourDesk/"
			if scenario == "service" {
				prefix = ""
			}
			for name, data := range files {
				fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(data), name)
				w, _ := z.Create(prefix + name)
				w.Write(data)
			}
			manifest := sums.String()
			if scenario == "tampered" {
				manifest = strings.Replace(manifest, manifest[:64], strings.Repeat("0", 64), 1)
			}
			w, _ := z.Create(prefix + "SHA256SUMS")
			w.Write([]byte(manifest))
			z.Close()
			file.Close()
			metadata, err := inspectLocalPortable(context.Background(), archive, dir, "amd64")
			valid := scenario == "valid" || scenario == "old"
			if (err == nil) != valid || valid && metadata.Version != version {
				t.Fatalf("套件驗證不符：%+v, %v", metadata, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "validated")); !os.IsNotExist(err) {
				t.Fatal("驗證後沒有清除解壓縮暫存")
			}
		})
	}
}

func TestLocalExistingPortableSmoke(t *testing.T) {
	archive := os.Getenv("YOURDESK_LOCAL_UPDATE_SMOKE_ZIP")
	if archive == "" {
		t.Skip("需指定待交付的 Portable ZIP")
	}
	arch := os.Getenv("YOURDESK_LOCAL_UPDATE_SMOKE_ARCH")
	if arch == "" {
		arch = "amd64"
	}
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	u := localManager(t)
	state, err := u.beginLocalUpdateForSystem(filepath.Base(archive), info.Size(), "windows")
	if err != nil {
		t.Fatal(err)
	}
	err = u.uploadLocalUpdate(context.Background(), state.Local.ID, file, func(ctx context.Context, staged, dir string) (localPackage, error) {
		return inspectLocalPortable(ctx, staged, dir, arch)
	})
	if err != nil {
		t.Fatal(err)
	}
	status := u.snapshot().Local
	if status.Phase != "ready" || versionKey(status.Version) == "" {
		t.Fatalf("正式套件未完成本地更新準備：%+v", status)
	}
	t.Logf("已驗證正式 %s Portable ZIP：%s", arch, status.Version)
	if err := u.cancelLocalUpdate(status.ID); err != nil {
		t.Fatal(err)
	}
}
