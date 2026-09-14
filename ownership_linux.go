//go:build linux

package svcforge

import (
	"os"
	"syscall"
)

func preserveOwnership(target string, info os.FileInfo, symlink bool) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if symlink {
		return os.Lchown(target, int(stat.Uid), int(stat.Gid))
	}
	return os.Chown(target, int(stat.Uid), int(stat.Gid))
}
