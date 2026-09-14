//go:build !linux

package svcforge

import "os"

func preserveOwnership(target string, info os.FileInfo, symlink bool) error {
	return nil
}
