//go:build windows

package repository

import "os"

// copyFileWithPermissions copies a file from src to dst with the same permissions.
func copyFileWithPermissions(src, dst string, _ os.FileInfo) error {
	return copyFile(src, dst)
}
