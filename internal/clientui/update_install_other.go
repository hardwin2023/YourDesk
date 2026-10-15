//go:build !darwin && !windows

package clientui

import (
	"context"
	"fmt"
	"os/exec"
)

func detachUpdateHelper(cmd *exec.Cmd) {}
func prepareAutomaticUpdate(ctx context.Context, path, _ string) error {
	return fmt.Errorf("此系統尚不支援自動安裝")
}
func inspectLocalUpdate(context.Context, string, string) (localPackage, error) {
	return localPackage{}, fmt.Errorf("此系統尚不支援本地更新。")
}
func prepareLocalUpdate(context.Context, string) error {
	return fmt.Errorf("此系統尚不支援本地更新。")
}
