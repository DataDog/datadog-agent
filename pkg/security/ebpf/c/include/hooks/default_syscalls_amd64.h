#ifndef _HOOKS_DEFAULT_SYSCALLS_AMD64_H
#define _HOOKS_DEFAULT_SYSCALLS_AMD64_H

// is_default_syscall returns 1 if id is in the baseline set of syscalls a
// typical Linux userspace process issues during normal operation (file I/O,
// memory management, signal handling, scheduling/synchronisation, process
// info, polling, process lifecycle). These are filtered out kernel-side to
// keep the per-cgroup syscall monitor map small. Must stay in sync with
// defaultSyscallSerializers in
// pkg/security/probe/monitors/syscalls/cgroup_monitor_linux_amd64.go.
static __attribute__((always_inline)) int is_default_syscall(unsigned long id) {
    switch (id) {
    // file I/O
    case 0:   // read
    case 1:   // write
    case 2:   // open
    case 3:   // close
    case 4:   // stat
    case 5:   // fstat
    case 6:   // lstat
    case 8:   // lseek
    case 16:  // ioctl
    case 17:  // pread64
    case 18:  // pwrite64
    case 19:  // readv
    case 20:  // writev
    case 72:  // fcntl
    case 78:  // getdents
    case 79:  // getcwd
    case 89:  // readlink
    case 217: // getdents64
    case 257: // openat
    case 262: // newfstatat
    case 267: // readlinkat
    case 437: // openat2
    // memory
    case 9:   // mmap
    case 10:  // mprotect
    case 11:  // munmap
    case 12:  // brk
    case 25:  // mremap
    // signals
    case 13:  // rt_sigaction
    case 14:  // rt_sigprocmask
    case 15:  // rt_sigreturn
    // scheduling / time / synchronisation
    case 24:  // sched_yield
    case 35:  // nanosleep
    case 202: // futex
    case 228: // clock_gettime
    case 318: // getrandom
    // process lifecycle
    case 56:  // clone
    case 57:  // fork
    case 58:  // vfork
    case 59:  // execve
    case 60:  // exit
    case 61:  // wait4
    case 231: // exit_group
    // process / user info
    case 39:  // getpid
    case 102: // getuid
    case 104: // getgid
    case 107: // geteuid
    case 108: // getegid
    case 110: // getppid
    case 186: // gettid
        return 1;
    }
    return 0;
}

#endif
