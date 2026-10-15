package prelogin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"time"
)

const desktopAccessUnavailable = "系統服務尚未支援獨立授權，請更新 APP 與服務後再試。"

var ErrDesktopAccessUnsupported = errors.New(desktopAccessUnavailable)

// 能力來自當前已驗證管線的握手，不從可跨版本殘留的磁碟標記推斷。
type desktopRuntime struct{ protocol atomic.Int32 }

func (r *desktopRuntime) hold(ctx context.Context, conn net.Conn, ready func()) error {
	r.protocol.Store(0)
	defer r.protocol.Store(0)
	return holdDesktopLease(ctx, conn, func() {
		r.protocol.Store(1)
		if ready != nil {
			ready()
		}
	})
}

func setInstalledDesktopAutostart(enabled bool, request func(bool) error, remove func() error) error {
	err := request(enabled)
	if err == nil || !errors.Is(err, ErrDesktopAccessUnsupported) {
		return err
	}
	if enabled {
		return ErrDesktopAccessUnsupported
	}
	// 只在已驗證的舊服務不認識操作，且使用者明確要求關閉時，走原本的移除。
	return remove()
}

func readDesktopReply(r io.Reader) error {
	return decodeDesktopReply(json.NewDecoder(io.LimitReader(r, 4096)))
}

func decodeDesktopReply(decoder *json.Decoder) error {
	var reply streamSettingsReply
	if err := decoder.Decode(&reply); err != nil {
		if errors.Is(err, io.EOF) {
			return ErrDesktopAccessUnsupported
		}
		return errors.New(desktopAccessUnavailable)
	}
	if reply.Error != "" {
		return errors.New(reply.Error)
	}
	if !reply.OK {
		return errors.New(desktopAccessUnavailable)
	}
	return nil
}

// 管線一旦完成握手，就只能維持租約；後續任何資料都會結束租約。
func holdDesktopLease(ctx context.Context, conn net.Conn, ready func()) error {
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline := time.Now().Add(3 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	if err := json.NewEncoder(conn).Encode(map[string]string{"operation": "desktop-lease"}); err != nil {
		return err
	}
	decoder := json.NewDecoder(io.LimitReader(conn, 4096))
	if err := decodeDesktopReply(decoder); err != nil {
		return err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if ready != nil {
		ready()
	}
	err := waitDesktopLease(io.MultiReader(decoder.Buffered(), conn))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func waitDesktopLease(input io.Reader) error {
	var data [64]byte
	for {
		n, err := input.Read(data[:])
		for _, c := range data[:n] {
			if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
				return errors.New("系統操作租約收到非預期資料。")
			}
		}
		if err != nil {
			return err
		}
	}
}

// 呼叫前必須驗證本機管線的來源與主控台工作階段。
func serveDesktopLease(ctx context.Context, conn net.Conn, input io.Reader, activity *desktopActivity, session uint32) {
	release, revoked, err := activity.hold(session)
	reply := streamSettingsReply{OK: err == nil}
	if err != nil {
		reply.Error = err.Error()
	} else {
		defer release()
		leaseCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			select {
			case <-revoked:
				cancel()
			case <-leaseCtx.Done():
			}
		}()
		stop := context.AfterFunc(leaseCtx, func() { _ = conn.Close() })
		defer stop()
	}
	if json.NewEncoder(conn).Encode(reply) != nil || err != nil {
		return
	}
	if conn.SetDeadline(time.Time{}) != nil {
		return
	}
	_ = waitDesktopLease(input)
}
