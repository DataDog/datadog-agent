// +build functionaltests,amd64 stresstests,amd64

package tests

import "syscall"

var supportedSyscalls = map[string]uintptr{
	"SYS_CHMOD":  syscall.SYS_CHMOD,
	"SYS_CHOWN":  syscall.SYS_CHOWN,
	"SYS_LCHOWN": syscall.SYS_LCHOWN,
	"SYS_LINK":   syscall.SYS_LINK,
	"SYS_MKDIR":  syscall.SYS_MKDIR,
	"SYS_OPEN":   syscall.SYS_OPEN,
	"SYS_CREAT":  syscall.SYS_CREAT,
	"SYS_RENAME": syscall.SYS_RENAME,
	"SYS_RMDIR":  syscall.SYS_RMDIR,
	"SYS_UNLINK": syscall.SYS_UNLINK,
	"SYS_UTIME":  syscall.SYS_UTIME,
	"SYS_UTIMES": syscall.SYS_UTIMES,
}
