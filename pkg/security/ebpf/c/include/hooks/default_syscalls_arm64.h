#ifndef _HOOKS_DEFAULT_SYSCALLS_ARM64_H
#define _HOOKS_DEFAULT_SYSCALLS_ARM64_H

// is_default_syscall returns 1 if id is in the baseline set of syscalls a
// typical Linux userspace process issues during normal operation (file I/O,
// memory management, signal handling, scheduling/synchronisation, process
// info, polling, process lifecycle). These are filtered out kernel-side to
// keep the per-cgroup syscall monitor map small.
//
// arm64 only exposes the newer "at"/"2" variants of several syscalls
// (openat instead of open, fstatat instead of stat/lstat/newfstatat,
// getdents64 instead of getdents, readlinkat instead of readlink, clone
// instead of fork/vfork). Must stay in sync with defaultSyscallSerializers
// in pkg/security/probe/monitors/syscalls/cgroup_monitor_linux_arm64.go.
static __attribute__((always_inline)) int is_default_syscall(unsigned long id) {
    switch (id) {
    // file I/O
    case 17:  // getcwd
    case 25:  // fcntl
    case 29:  // ioctl
    case 56:  // openat
    case 57:  // close
    case 61:  // getdents64
    case 62:  // lseek
    case 63:  // read
    case 64:  // write
    case 65:  // readv
    case 66:  // writev
    case 67:  // pread64
    case 68:  // pwrite64
    case 78:  // readlinkat
    case 79:  // fstatat
    case 80:  // fstat
    case 437: // openat2
    // process lifecycle
    case 93:  // exit
    case 94:  // exit_group
    case 220: // clone
    case 221: // execve
    case 260: // wait4
    case 281: // execveat
    // scheduling / time / synchronisation
    case 98:  // futex
    case 101: // nanosleep
    case 113: // clock_gettime
    case 124: // sched_yield
    case 278: // getrandom
    // signals
    case 134: // rt_sigaction
    case 135: // rt_sigprocmask
    case 139: // rt_sigreturn
    // process / user info
    case 172: // getpid
    case 173: // getppid
    case 174: // getuid
    case 175: // geteuid
    case 176: // getgid
    case 177: // getegid
    case 178: // gettid
    // memory
    case 214: // brk
    case 215: // munmap
    case 216: // mremap
    case 222: // mmap
    case 226: // mprotect
        return 1;
    }
    return 0;
}

#endif
