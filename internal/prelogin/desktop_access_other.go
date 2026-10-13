//go:build !windows

package prelogin

import (
	"context"
	"errors"
)

func ConfigureDesktopAccess(context.Context, bool, Config) error {
	return errors.New("此平台不支援系統操作授權。")
}
func installDesktopAccess(string) error { return errors.New("此平台不支援系統操作授權。") }
func HoldDesktopAccess(context.Context, func()) error {
	return errors.New("此平台不支援系統操作授權。")
}
