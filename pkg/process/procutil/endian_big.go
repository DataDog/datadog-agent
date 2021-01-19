// this is copied from https://golang.org/src/syscall/endian_little.go,
// so far procutil is used only for linux so we have "linux" build tag for each architecture,
// otherwise the compilation on some platforms will get "`isBigEndian` is unused (deadcode)" error

// +build linux,ppc64 linux,s390x linux,mips linux,mips64

package procutil

const isBigEndian = true
