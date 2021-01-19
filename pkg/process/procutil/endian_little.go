// this is copied from https://golang.org/src/syscall/endian_little.go,
// so far procutil is used only for linux so we have "linux" build tag for each architecture,
// otherwise the compilation on some platforms will get "`isBigEndian` is unused (deadcode)" error

// +build linux,386 linux,amd64 linux,arm linux,arm64 linux,ppc64le linux,mips64le linux,mipsle linux,riscv64 linux,wasm

package procutil

const isBigEndian = false
