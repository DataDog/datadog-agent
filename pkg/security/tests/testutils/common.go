package testutils

import "golang.org/x/sys/unix"

func SyscallExists(syscall int) bool {
	ret, _, err := unix.Syscall(uintptr(syscall), 0, 0, 0)
	if int(ret) == -1 && err == unix.ENOSYS {
		return false
	}
	return true
}
