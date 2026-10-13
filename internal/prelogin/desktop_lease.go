package prelogin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"
)

const desktopAccessUnavailable = "系統服務尚未支援獨立授權，請更新 APP 與服務後再試。"

func readDesktopReply(r io.Reader) error {
	var reply streamSettingsReply
	if err := json.NewDecoder(io.LimitReader(r, 4096)).Decode(&reply); err != nil {
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
	if err := readDesktopReply(conn); err != nil {
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
	var data [1]byte
	_, err := conn.Read(data[:])
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	return errors.New("系統操作租約收到非預期資料。")
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
	var data [1]byte
	for {
		// JSON 握手後允許空白分隔；不接受第二個要求或額外操作。
		if n, err := input.Read(data[:]); err != nil || (n > 0 && data[0] != ' ' && data[0] != '\t' && data[0] != '\r' && data[0] != '\n') {
			return
		}
	}
}
