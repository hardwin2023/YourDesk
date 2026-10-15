package clientui

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const localUpdateTimeout = 10 * time.Minute

type localUpdateStatus struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Size                  int64  `json:"size"`
	Bytes                 int64  `json:"bytes"`
	Phase                 string `json:"phase"`
	Version               string `json:"version,omitempty"`
	Architecture          string `json:"architecture,omitempty"`
	RequiresAuthorization bool   `json:"requiresAuthorization,omitempty"`
	Error                 string `json:"error,omitempty"`
}

type localPackage struct {
	Version               string
	RequiresAuthorization bool
}

// 使用者選取的檔案只寫入獨立私有目錄，不接受任意本機路徑或 URL。
// 確認安裝前完成內容驗證；上傳與安裝共用同一份固定副本。
type localUpdate struct {
	status  localUpdateStatus // 由 updateManager.mu 保護。
	dir     string
	archive string
	digest  string
	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup
}

func localUpdateExtension(system string) string {
	switch system {
	case "windows":
		return ".zip"
	case "darwin":
		return ".dmg"
	default:
		return ""
	}
}

func localUpdateBusy(status *localUpdateStatus) bool {
	return status != nil && status.Phase != "failed"
}
func (u *updateManager) localBusyLocked() bool {
	return u.local != nil && localUpdateBusy(&u.local.status)
}

func (u *updateManager) beginLocalUpdate(name string, size int64) (updateStatus, error) {
	return u.beginLocalUpdateForSystem(name, size, runtime.GOOS)
}

func (u *updateManager) beginLocalUpdateForSystem(name string, size int64, system string) (updateStatus, error) {
	ext := localUpdateExtension(system)
	if ext == "" {
		return updateStatus{}, errors.New("此系統尚不支援本地更新。")
	}
	if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\:\x00\r\n") || !strings.EqualFold(filepath.Ext(name), ext) {
		return updateStatus{}, fmt.Errorf("請選擇此系統的 YourDesk %s 更新套件。", strings.ToUpper(ext))
	}
	if size <= 0 || size > maxUpdateSize {
		return updateStatus{}, errors.New("更新套件必須介於 1 byte 與 2 GB。")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.ctx.Err() != nil || u.state.Downloading || u.state.Opening || u.state.InstallAt > 0 || u.localBusyLocked() {
		return updateStatus{}, errors.New("已有更新進行中，請先完成或取消。")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return updateStatus{}, err
	}
	dir, err := os.MkdirTemp("", "YourDesk-local-update-")
	if err != nil {
		return updateStatus{}, fmt.Errorf("無法建立更新暫存目錄：%w", err)
	}
	if u.local != nil {
		u.local.cancel()
	}
	ctx, cancel := context.WithCancel(u.ctx)
	job := &localUpdate{
		status: localUpdateStatus{ID: hex.EncodeToString(id), Name: name, Size: size, Phase: "selected"},
		dir:    dir, archive: filepath.Join(dir, "package"+ext), ctx: ctx, cancel: cancel,
	}
	u.local = job
	// 即使關閉視窗／結束程式，也先等待該工作的讀寫完成再清除暫存。
	u.downloads.Add(1)
	go func() {
		defer u.downloads.Done()
		<-ctx.Done()
		u.mu.Lock() // 與 workers.Add 及取消決定排序。
		u.mu.Unlock()
		job.workers.Wait()
		os.RemoveAll(job.dir)
	}()
	return u.snapshotLocked(), nil
}

func (u *updateManager) cancelLocalUpdate(id string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.local == nil || id == "" || u.local.status.ID != id {
		return errors.New("本地更新已取消或已過期，請重新選擇套件。")
	}
	if u.local.status.Phase == "installing" {
		return errors.New("更新交接中，請等待程式重新啟動。")
	}
	u.local.cancel()
	u.local = nil
	return nil
}

func (u *updateManager) uploadLocalUpdate(ctx context.Context, id string, body io.ReadCloser, inspect func(context.Context, string, string) (localPackage, error)) error {
	u.mu.Lock()
	job := u.local
	if job == nil || id == "" || job.status.ID != id || job.status.Phase != "selected" || job.ctx.Err() != nil {
		u.mu.Unlock()
		return errors.New("本地更新已取消或已過期，請重新選擇套件。")
	}
	job.status.Phase = "uploading"
	job.workers.Add(1)
	u.mu.Unlock()
	defer job.workers.Done()
	ctx, cancel := context.WithTimeout(ctx, localUpdateTimeout)
	defer cancel()
	stop := context.AfterFunc(job.ctx, cancel)
	defer stop()
	stopRead := context.AfterFunc(ctx, func() { body.Close() })
	defer stopRead()
	metadata, digest, err := func() (localPackage, string, error) {
		file, err := os.OpenFile(job.archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return localPackage{}, "", fmt.Errorf("無法儲存更新套件：%w", err)
		}
		hash := sha256.New()
		progress := &localUploadProgress{u: u, job: job}
		n, copyErr := io.Copy(io.MultiWriter(file, hash, progress), io.LimitReader(body, job.status.Size+1))
		closeErr := file.Close()
		if copyErr != nil {
			return localPackage{}, "", fmt.Errorf("更新套件上傳失敗：%w", copyErr)
		}
		if ctx.Err() != nil {
			return localPackage{}, "", ctx.Err()
		}
		if n != job.status.Size {
			return localPackage{}, "", errors.New("更新套件大小不符，請重新選擇檔案。")
		}
		if closeErr != nil {
			return localPackage{}, "", closeErr
		}
		u.mu.Lock()
		job.status.Phase = "validating"
		u.mu.Unlock()
		metadata, err := inspect(ctx, job.archive, job.dir)
		if err == nil && versionKey(metadata.Version) == "" {
			err = errors.New("更新套件的版本資訊無效。")
		}
		// 本地更新由使用者選定版本，允許升版、同版修復及回退。
		return metadata, hex.EncodeToString(hash.Sum(nil)), err
	}()
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.local != job || job.ctx.Err() != nil {
		return errors.New("本地更新已取消或已過期，請重新選擇套件。")
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		job.status.Phase, job.status.Error = "failed", err.Error()
		job.cancel()
		return err
	}
	job.digest = digest
	job.status.Phase, job.status.Version = "ready", metadata.Version
	job.status.Architecture = runtime.GOARCH
	job.status.RequiresAuthorization = metadata.RequiresAuthorization
	return nil
}

type localUploadProgress struct {
	u   *updateManager
	job *localUpdate
}

type localUpdateBody struct {
	io.ReadCloser
	unblock func()
}

func (b *localUpdateBody) Close() error {
	// net/http 的 Body.Close 可能等待正在進行的 Read，先解除 socket 讀取。
	b.unblock()
	return b.ReadCloser.Close()
}

func (p *localUploadProgress) Write(data []byte) (int, error) {
	p.u.mu.Lock()
	p.job.status.Bytes = min(p.job.status.Size, p.job.status.Bytes+int64(len(data)))
	p.u.mu.Unlock()
	return len(data), p.job.ctx.Err()
}

func (u *updateManager) installLocalUpdate(id string, prepare func(context.Context, string) error, onOpened func()) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	job := u.local
	if job == nil || id == "" || job.status.ID != id || job.status.Phase != "ready" || job.ctx.Err() != nil {
		return errors.New("套件尚未驗證完成，或本地更新已取消。")
	}
	job.status.Phase = "installing"
	job.workers.Add(1)
	go func() {
		defer job.workers.Done()
		err := verifyLocalArchive(job.ctx, job.archive, job.status.Size, job.digest)
		if err == nil {
			err = prepare(job.ctx, job.archive)
		}
		u.mu.Lock()
		if err != nil {
			job.status.Phase, job.status.Error = "failed", "本地更新準備失敗："+err.Error()
		} else {
			// 新版首次啟動時讀取套件隨附的更新重點；失敗不覆蓋 OTA 狀態。
			u.state.ShowNotes, u.state.NotesVersion = true, job.status.Version
			u.state.Notes, u.state.NotesTest = nil, false
			u.saveLocked()
		}
		u.mu.Unlock()
		job.cancel()
		if err == nil && onOpened != nil {
			onOpened()
		}
	}()
	return nil
}

func verifyLocalArchive(ctx context.Context, name string, size int64, digest string) error {
	invalid := errors.New("更新暫存套件遺失或內容已變更，請重新選擇檔案。")
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return invalid
	}
	file, err := os.Open(name)
	if err != nil {
		return invalid
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, 128<<10)
	for ctx.Err() == nil {
		n, err := file.Read(buffer)
		hash.Write(buffer[:n])
		if err == io.EOF {
			if hex.EncodeToString(hash.Sum(nil)) == digest {
				return nil
			}
			return invalid
		}
		if err != nil {
			return invalid
		}
	}
	return ctx.Err()
}

func inspectLocalPortable(ctx context.Context, archive, dir, arch string) (localPackage, error) {
	payload := filepath.Join(dir, "validated")
	defer os.RemoveAll(payload)
	if _, err := extractPortableUpdate(ctx, archive, payload, arch); err != nil {
		return localPackage{}, fmt.Errorf("Portable ZIP 驗證失敗：%w", err)
	}
	file, err := os.Open(filepath.Join(payload, "README.txt"))
	if err != nil {
		return localPackage{}, errors.New("更新套件缺少版本說明，請選擇完整 Portable ZIP。")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 4096))
	if err != nil {
		return localPackage{}, err
	}
	line := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(string(data), "\ufeff"), "\n", 2)[0])
	if !strings.HasPrefix(line, "YourDesk ") || versionKey(strings.TrimPrefix(line, "YourDesk ")) == "" {
		return localPackage{}, errors.New("更新套件的版本資訊無效。")
	}
	return localPackage{Version: strings.TrimPrefix(line, "YourDesk ")}, nil
}

func (s *server) handleLocalUpdates(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/updates/local" && r.URL.Path != "/api/updates/local/install" && r.URL.Path != "/api/updates/local/cancel" {
		return false
	}
	u := s.updater
	switch {
	case r.URL.Path == "/api/updates/local" && r.Method == "POST":
		var request struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		}
		if err := decode(w, r, &request); err != nil {
			fail(w, err)
		} else if state, err := u.beginLocalUpdate(request.Name, request.Size); err != nil {
			fail(w, err)
		} else {
			respond(w, 200, state)
		}
	case r.URL.Path == "/api/updates/local" && r.Method == "PUT":
		// 一般 API 的 15 秒期限不適用大型本機檔案；仍限制大小及總時長。
		deadline := time.Now().Add(localUpdateTimeout)
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(deadline)
		_ = controller.SetWriteDeadline(deadline.Add(5 * time.Second))
		defer r.Body.Close()
		body := &localUpdateBody{ReadCloser: r.Body, unblock: func() { _ = controller.SetReadDeadline(time.Now()) }}
		if err := u.uploadLocalUpdate(r.Context(), r.URL.Query().Get("id"), body, inspectLocalUpdate); err != nil {
			fail(w, err)
		} else {
			respond(w, 200, u.snapshot())
		}
	case (r.URL.Path == "/api/updates/local/install" || r.URL.Path == "/api/updates/local/cancel") && r.Method == "POST":
		var request struct {
			ID string `json:"id"`
		}
		if err := decode(w, r, &request); err != nil {
			fail(w, err)
			break
		}
		var err error
		if r.URL.Path == "/api/updates/local/cancel" {
			err = u.cancelLocalUpdate(request.ID)
		} else if s.closeApplication == nil {
			err = errors.New("無法結束本程式")
		} else {
			err = u.installLocalUpdate(request.ID, prepareLocalUpdate, s.closeApplication)
		}
		if err != nil {
			fail(w, err)
		} else {
			respond(w, 200, u.snapshot())
		}
	default:
		respond(w, http.StatusMethodNotAllowed, map[string]string{"error": "不支援的操作"})
	}
	return true
}
