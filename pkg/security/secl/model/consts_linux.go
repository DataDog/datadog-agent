// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"fmt"
	"math"
	"math/bits"
	"sort"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/sys/unix"
)

var (
	// errorConstants are the supported error constants
	// generate_constants:Error constants,Error constants are the supported error constants.
	errorConstants = map[string]int{
		"E2BIG":           -int(syscall.E2BIG),
		"EACCES":          -int(syscall.EACCES),
		"EADDRINUSE":      -int(syscall.EADDRINUSE),
		"EADDRNOTAVAIL":   -int(syscall.EADDRNOTAVAIL),
		"EADV":            -int(syscall.EADV),
		"EAFNOSUPPORT":    -int(syscall.EAFNOSUPPORT),
		"EAGAIN":          -int(syscall.EAGAIN),
		"EALREADY":        -int(syscall.EALREADY),
		"EBADE":           -int(syscall.EBADE),
		"EBADF":           -int(syscall.EBADF),
		"EBADFD":          -int(syscall.EBADFD),
		"EBADMSG":         -int(syscall.EBADMSG),
		"EBADR":           -int(syscall.EBADR),
		"EBADRQC":         -int(syscall.EBADRQC),
		"EBADSLT":         -int(syscall.EBADSLT),
		"EBFONT":          -int(syscall.EBFONT),
		"EBUSY":           -int(syscall.EBUSY),
		"ECANCELED":       -int(syscall.ECANCELED),
		"ECHILD":          -int(syscall.ECHILD),
		"ECHRNG":          -int(syscall.ECHRNG),
		"ECOMM":           -int(syscall.ECOMM),
		"ECONNABORTED":    -int(syscall.ECONNABORTED),
		"ECONNREFUSED":    -int(syscall.ECONNREFUSED),
		"ECONNRESET":      -int(syscall.ECONNRESET),
		"EDEADLK":         -int(syscall.EDEADLK),
		"EDEADLOCK":       -int(syscall.EDEADLOCK),
		"EDESTADDRREQ":    -int(syscall.EDESTADDRREQ),
		"EDOM":            -int(syscall.EDOM),
		"EDOTDOT":         -int(syscall.EDOTDOT),
		"EDQUOT":          -int(syscall.EDQUOT),
		"EEXIST":          -int(syscall.EEXIST),
		"EFAULT":          -int(syscall.EFAULT),
		"EFBIG":           -int(syscall.EFBIG),
		"EHOSTDOWN":       -int(syscall.EHOSTDOWN),
		"EHOSTUNREACH":    -int(syscall.EHOSTUNREACH),
		"EIDRM":           -int(syscall.EIDRM),
		"EILSEQ":          -int(syscall.EIDRM),
		"EINPROGRESS":     -int(syscall.EINPROGRESS),
		"EINTR":           -int(syscall.EINTR),
		"EINVAL":          -int(syscall.EINVAL),
		"EIO":             -int(syscall.EIO),
		"EISCONN":         -int(syscall.EISCONN),
		"EISDIR":          -int(syscall.EISDIR),
		"EISNAM":          -int(syscall.EISNAM),
		"EKEYEXPIRED":     -int(syscall.EKEYEXPIRED),
		"EKEYREJECTED":    -int(syscall.EKEYREJECTED),
		"EKEYREVOKED":     -int(syscall.EKEYREVOKED),
		"EL2HLT":          -int(syscall.EL2HLT),
		"EL2NSYNC":        -int(syscall.EL2NSYNC),
		"EL3HLT":          -int(syscall.EL3HLT),
		"EL3RST":          -int(syscall.EL3RST),
		"ELIBACC":         -int(syscall.ELIBACC),
		"ELIBBAD":         -int(syscall.ELIBBAD),
		"ELIBEXEC":        -int(syscall.ELIBEXEC),
		"ELIBMAX":         -int(syscall.ELIBMAX),
		"ELIBSCN":         -int(syscall.ELIBSCN),
		"ELNRNG":          -int(syscall.ELNRNG),
		"ELOOP":           -int(syscall.ELOOP),
		"EMEDIUMTYPE":     -int(syscall.EMEDIUMTYPE),
		"EMFILE":          -int(syscall.EMFILE),
		"EMLINK":          -int(syscall.EMLINK),
		"EMSGSIZE":        -int(syscall.EMSGSIZE),
		"EMULTIHOP":       -int(syscall.EMULTIHOP),
		"ENAMETOOLONG":    -int(syscall.ENAMETOOLONG),
		"ENAVAIL":         -int(syscall.ENAVAIL),
		"ENETDOWN":        -int(syscall.ENETDOWN),
		"ENETRESET":       -int(syscall.ENETRESET),
		"ENETUNREACH":     -int(syscall.ENETUNREACH),
		"ENFILE":          -int(syscall.ENFILE),
		"ENOANO":          -int(syscall.ENOANO),
		"ENOBUFS":         -int(syscall.ENOBUFS),
		"ENOCSI":          -int(syscall.ENOCSI),
		"ENODATA":         -int(syscall.ENODATA),
		"ENODEV":          -int(syscall.ENODEV),
		"ENOENT":          -int(syscall.ENOENT),
		"ENOEXEC":         -int(syscall.ENOEXEC),
		"ENOKEY":          -int(syscall.ENOKEY),
		"ENOLCK":          -int(syscall.ENOLCK),
		"ENOLINK":         -int(syscall.ENOLINK),
		"ENOMEDIUM":       -int(syscall.ENOMEDIUM),
		"ENOMEM":          -int(syscall.ENOMEM),
		"ENOMSG":          -int(syscall.ENOMSG),
		"ENONET":          -int(syscall.ENONET),
		"ENOPKG":          -int(syscall.ENOPKG),
		"ENOPROTOOPT":     -int(syscall.ENOPROTOOPT),
		"ENOSPC":          -int(syscall.ENOSPC),
		"ENOSR":           -int(syscall.ENOSR),
		"ENOSTR":          -int(syscall.ENOSTR),
		"ENOSYS":          -int(syscall.ENOSYS),
		"ENOTBLK":         -int(syscall.ENOTBLK),
		"ENOTCONN":        -int(syscall.ENOTCONN),
		"ENOTDIR":         -int(syscall.ENOTDIR),
		"ENOTEMPTY":       -int(syscall.ENOTEMPTY),
		"ENOTNAM":         -int(syscall.ENOTNAM),
		"ENOTRECOVERABLE": -int(syscall.ENOTRECOVERABLE),
		"ENOTSOCK":        -int(syscall.ENOTSOCK),
		"ENOTSUP":         -int(syscall.ENOTSUP),
		"ENOTTY":          -int(syscall.ENOTTY),
		"ENOTUNIQ":        -int(syscall.ENOTUNIQ),
		"ENXIO":           -int(syscall.ENXIO),
		"EOPNOTSUPP":      -int(syscall.EOPNOTSUPP),
		"EOVERFLOW":       -int(syscall.EOVERFLOW),
		"EOWNERDEAD":      -int(syscall.EOWNERDEAD),
		"EPERM":           -int(syscall.EPERM),
		"EPFNOSUPPORT":    -int(syscall.EPFNOSUPPORT),
		"EPIPE":           -int(syscall.EPIPE),
		"EPROTO":          -int(syscall.EPROTO),
		"EPROTONOSUPPORT": -int(syscall.EPROTONOSUPPORT),
		"EPROTOTYPE":      -int(syscall.EPROTOTYPE),
		"ERANGE":          -int(syscall.ERANGE),
		"EREMCHG":         -int(syscall.EREMCHG),
		"EREMOTE":         -int(syscall.EREMOTE),
		"EREMOTEIO":       -int(syscall.EREMOTEIO),
		"ERESTART":        -int(syscall.ERESTART),
		"ERFKILL":         -int(syscall.ERFKILL),
		"EROFS":           -int(syscall.EROFS),
		"ESHUTDOWN":       -int(syscall.ESHUTDOWN),
		"ESOCKTNOSUPPORT": -int(syscall.ESOCKTNOSUPPORT),
		"ESPIPE":          -int(syscall.ESPIPE),
		"ESRCH":           -int(syscall.ESRCH),
		"ESRMNT":          -int(syscall.ESRMNT),
		"ESTALE":          -int(syscall.ESTALE),
		"ESTRPIPE":        -int(syscall.ESTRPIPE),
		"ETIME":           -int(syscall.ETIME),
		"ETIMEDOUT":       -int(syscall.ETIMEDOUT),
		"ETOOMANYREFS":    -int(syscall.ETOOMANYREFS),
		"ETXTBSY":         -int(syscall.ETXTBSY),
		"EUCLEAN":         -int(syscall.EUCLEAN),
		"EUNATCH":         -int(syscall.EUNATCH),
		"EUSERS":          -int(syscall.EUSERS),
		"EWOULDBLOCK":     -int(syscall.EWOULDBLOCK),
		"EXDEV":           -int(syscall.EXDEV),
		"EXFULL":          -int(syscall.EXFULL),
	}

	// openFlagsConstants are the supported flags for the open syscall
	// generate_constants:Open flags,Open flags are the supported flags for the open syscall.
	openFlagsConstants = map[string]int{
		"O_RDONLY":    syscall.O_RDONLY,
		"O_WRONLY":    syscall.O_WRONLY,
		"O_RDWR":      syscall.O_RDWR,
		"O_APPEND":    syscall.O_APPEND,
		"O_CREAT":     syscall.O_CREAT,
		"O_EXCL":      syscall.O_EXCL,
		"O_SYNC":      syscall.O_SYNC,
		"O_TRUNC":     syscall.O_TRUNC,
		"O_ACCMODE":   syscall.O_ACCMODE,
		"O_ASYNC":     syscall.O_ASYNC,
		"O_CLOEXEC":   syscall.O_CLOEXEC,
		"O_DIRECT":    syscall.O_DIRECT,
		"O_DIRECTORY": syscall.O_DIRECTORY,
		"O_DSYNC":     syscall.O_DSYNC,
		"O_FSYNC":     syscall.O_FSYNC,
		// "O_LARGEFILE": syscall.O_LARGEFILE, golang defines this as 0
		"O_NDELAY":   syscall.O_NDELAY,
		"O_NOATIME":  syscall.O_NOATIME,
		"O_NOCTTY":   syscall.O_NOCTTY,
		"O_NOFOLLOW": syscall.O_NOFOLLOW,
		"O_NONBLOCK": syscall.O_NONBLOCK,
		"O_RSYNC":    syscall.O_RSYNC,
	}

	// fileModeConstants contains the constants describing file permissions as well as the set-user-ID, set-group-ID, and sticky bits.
	// generate_constants:File mode constants,File mode constants are the supported file permissions as well as constants for the set-user-ID, set-group-ID, and sticky bits.
	fileModeConstants = map[string]int{
		// "S_IREAD":  syscall.S_IREAD, deprecated
		"S_ISUID": syscall.S_ISUID,
		"S_ISGID": syscall.S_ISGID,
		"S_ISVTX": syscall.S_ISVTX,
		"S_IRWXU": syscall.S_IRWXU,
		"S_IRUSR": syscall.S_IRUSR,
		"S_IWUSR": syscall.S_IWUSR,
		"S_IXUSR": syscall.S_IXUSR,
		"S_IRWXG": syscall.S_IRWXG,
		"S_IRGRP": syscall.S_IRGRP,
		"S_IWGRP": syscall.S_IWGRP,
		"S_IXGRP": syscall.S_IXGRP,
		"S_IRWXO": syscall.S_IRWXO,
		"S_IROTH": syscall.S_IROTH,
		"S_IWOTH": syscall.S_IWOTH,
		"S_IXOTH": syscall.S_IXOTH,
		// "S_IWRITE": syscall.S_IWRITE, deprecated
	}

	// inodeModeConstants are the supported file types and file modes
	// generate_constants:Inode mode constants,Inode mode constants are the supported file type constants as well as the file mode constants.
	inodeModeConstants = map[string]int{
		// "S_IEXEC":  syscall.S_IEXEC, deprecated
		"S_IFMT":   syscall.S_IFMT,
		"S_IFSOCK": syscall.S_IFSOCK,
		"S_IFLNK":  syscall.S_IFLNK,
		"S_IFREG":  syscall.S_IFREG,
		"S_IFBLK":  syscall.S_IFBLK,
		"S_IFDIR":  syscall.S_IFDIR,
		"S_IFCHR":  syscall.S_IFCHR,
		"S_IFIFO":  syscall.S_IFIFO,
		"S_ISUID":  syscall.S_ISUID,
		"S_ISGID":  syscall.S_ISGID,
		"S_ISVTX":  syscall.S_ISVTX,
		"S_IRWXU":  syscall.S_IRWXU,
		"S_IRUSR":  syscall.S_IRUSR,
		"S_IWUSR":  syscall.S_IWUSR,
		"S_IXUSR":  syscall.S_IXUSR,
		"S_IRWXG":  syscall.S_IRWXG,
		"S_IRGRP":  syscall.S_IRGRP,
		"S_IWGRP":  syscall.S_IWGRP,
		"S_IXGRP":  syscall.S_IXGRP,
		"S_IRWXO":  syscall.S_IRWXO,
		"S_IROTH":  syscall.S_IROTH,
		"S_IWOTH":  syscall.S_IWOTH,
		"S_IXOTH":  syscall.S_IXOTH,
	}

	// KernelCapabilityConstants list of kernel capabilities
	// generate_constants:Kernel Capability constants,Kernel Capability constants are the supported Linux Kernel Capability.
	KernelCapabilityConstants = map[string]uint64{
		"CAP_AUDIT_CONTROL":      1 << unix.CAP_AUDIT_CONTROL,
		"CAP_AUDIT_READ":         1 << unix.CAP_AUDIT_READ,
		"CAP_AUDIT_WRITE":        1 << unix.CAP_AUDIT_WRITE,
		"CAP_BLOCK_SUSPEND":      1 << unix.CAP_BLOCK_SUSPEND,
		"CAP_BPF":                1 << unix.CAP_BPF,
		"CAP_CHECKPOINT_RESTORE": 1 << unix.CAP_CHECKPOINT_RESTORE,
		"CAP_CHOWN":              1 << unix.CAP_CHOWN,
		"CAP_DAC_OVERRIDE":       1 << unix.CAP_DAC_OVERRIDE,
		"CAP_DAC_READ_SEARCH":    1 << unix.CAP_DAC_READ_SEARCH,
		"CAP_FOWNER":             1 << unix.CAP_FOWNER,
		"CAP_FSETID":             1 << unix.CAP_FSETID,
		"CAP_IPC_LOCK":           1 << unix.CAP_IPC_LOCK,
		"CAP_IPC_OWNER":          1 << unix.CAP_IPC_OWNER,
		"CAP_KILL":               1 << unix.CAP_KILL,
		"CAP_LEASE":              1 << unix.CAP_LEASE,
		"CAP_LINUX_IMMUTABLE":    1 << unix.CAP_LINUX_IMMUTABLE,
		"CAP_MAC_ADMIN":          1 << unix.CAP_MAC_ADMIN,
		"CAP_MAC_OVERRIDE":       1 << unix.CAP_MAC_OVERRIDE,
		"CAP_MKNOD":              1 << unix.CAP_MKNOD,
		"CAP_NET_ADMIN":          1 << unix.CAP_NET_ADMIN,
		"CAP_NET_BIND_SERVICE":   1 << unix.CAP_NET_BIND_SERVICE,
		"CAP_NET_BROADCAST":      1 << unix.CAP_NET_BROADCAST,
		"CAP_NET_RAW":            1 << unix.CAP_NET_RAW,
		"CAP_PERFMON":            1 << unix.CAP_PERFMON,
		"CAP_SETFCAP":            1 << unix.CAP_SETFCAP,
		"CAP_SETGID":             1 << unix.CAP_SETGID,
		"CAP_SETPCAP":            1 << unix.CAP_SETPCAP,
		"CAP_SETUID":             1 << unix.CAP_SETUID,
		"CAP_SYSLOG":             1 << unix.CAP_SYSLOG,
		"CAP_SYS_ADMIN":          1 << unix.CAP_SYS_ADMIN,
		"CAP_SYS_BOOT":           1 << unix.CAP_SYS_BOOT,
		"CAP_SYS_CHROOT":         1 << unix.CAP_SYS_CHROOT,
		"CAP_SYS_MODULE":         1 << unix.CAP_SYS_MODULE,
		"CAP_SYS_NICE":           1 << unix.CAP_SYS_NICE,
		"CAP_SYS_PACCT":          1 << unix.CAP_SYS_PACCT,
		"CAP_SYS_PTRACE":         1 << unix.CAP_SYS_PTRACE,
		"CAP_SYS_RAWIO":          1 << unix.CAP_SYS_RAWIO,
		"CAP_SYS_RESOURCE":       1 << unix.CAP_SYS_RESOURCE,
		"CAP_SYS_TIME":           1 << unix.CAP_SYS_TIME,
		"CAP_SYS_TTY_CONFIG":     1 << unix.CAP_SYS_TTY_CONFIG,
		"CAP_WAKE_ALARM":         1 << unix.CAP_WAKE_ALARM,
	}

	// ptraceConstants are the supported ptrace commands for the ptrace syscall
	// generate_constants:Ptrace constants,Ptrace constants are the supported ptrace commands for the ptrace syscall.
	ptraceConstants = map[string]uint32{
		"PTRACE_TRACEME":    unix.PTRACE_TRACEME,
		"PTRACE_PEEKTEXT":   unix.PTRACE_PEEKTEXT,
		"PTRACE_PEEKDATA":   unix.PTRACE_PEEKDATA,
		"PTRACE_PEEKUSR":    unix.PTRACE_PEEKUSR,
		"PTRACE_POKETEXT":   unix.PTRACE_POKETEXT,
		"PTRACE_POKEDATA":   unix.PTRACE_POKEDATA,
		"PTRACE_POKEUSR":    unix.PTRACE_POKEUSR,
		"PTRACE_CONT":       unix.PTRACE_CONT,
		"PTRACE_KILL":       unix.PTRACE_KILL,
		"PTRACE_SINGLESTEP": unix.PTRACE_SINGLESTEP,
		"PTRACE_ATTACH":     unix.PTRACE_ATTACH,
		"PTRACE_DETACH":     unix.PTRACE_DETACH,
		"PTRACE_SYSCALL":    unix.PTRACE_SYSCALL,

		"PTRACE_SETOPTIONS":           unix.PTRACE_SETOPTIONS,
		"PTRACE_GETEVENTMSG":          unix.PTRACE_GETEVENTMSG,
		"PTRACE_GETSIGINFO":           unix.PTRACE_GETSIGINFO,
		"PTRACE_SETSIGINFO":           unix.PTRACE_SETSIGINFO,
		"PTRACE_GETREGSET":            unix.PTRACE_GETREGSET,
		"PTRACE_SETREGSET":            unix.PTRACE_SETREGSET,
		"PTRACE_SEIZE":                unix.PTRACE_SEIZE,
		"PTRACE_INTERRUPT":            unix.PTRACE_INTERRUPT,
		"PTRACE_LISTEN":               unix.PTRACE_LISTEN,
		"PTRACE_PEEKSIGINFO":          unix.PTRACE_PEEKSIGINFO,
		"PTRACE_GETSIGMASK":           unix.PTRACE_GETSIGMASK,
		"PTRACE_SETSIGMASK":           unix.PTRACE_SETSIGMASK,
		"PTRACE_SECCOMP_GET_FILTER":   unix.PTRACE_SECCOMP_GET_FILTER,
		"PTRACE_SECCOMP_GET_METADATA": unix.PTRACE_SECCOMP_GET_METADATA,
		"PTRACE_GET_SYSCALL_INFO":     unix.PTRACE_GET_SYSCALL_INFO,
	}

	// protConstants are the supported protections for the mmap syscall
	// generate_constants:Protection constants,Protection constants are the supported protections for the mmap syscall.
	protConstants = map[string]uint64{
		"PROT_NONE":      unix.PROT_NONE,
		"PROT_READ":      unix.PROT_READ,
		"PROT_WRITE":     unix.PROT_WRITE,
		"PROT_EXEC":      unix.PROT_EXEC,
		"PROT_GROWSDOWN": unix.PROT_GROWSDOWN,
		"PROT_GROWSUP":   unix.PROT_GROWSUP,
	}

	// mmapFlagConstants are the supported flags for the mmap syscall
	// generate_constants:MMap flags,MMap flags are the supported flags for the mmap syscall.
	mmapFlagConstants = map[string]uint64{
		"MAP_SHARED":          unix.MAP_SHARED,          /* Share changes */
		"MAP_PRIVATE":         unix.MAP_PRIVATE,         /* Changes are private */
		"MAP_SHARED_VALIDATE": unix.MAP_SHARED_VALIDATE, /* share + validate extension flags */
		"MAP_ANON":            unix.MAP_ANON,
		"MAP_ANONYMOUS":       unix.MAP_ANONYMOUS,       /* don't use a file */
		"MAP_DENYWRITE":       unix.MAP_DENYWRITE,       /* ETXTBSY */
		"MAP_EXECUTABLE":      unix.MAP_EXECUTABLE,      /* mark it as an executable */
		"MAP_FIXED":           unix.MAP_FIXED,           /* Interpret addr exactly */
		"MAP_FIXED_NOREPLACE": unix.MAP_FIXED_NOREPLACE, /* MAP_FIXED which doesn't unmap underlying mapping */
		"MAP_GROWSDOWN":       unix.MAP_GROWSDOWN,       /* stack-like segment */
		"MAP_HUGETLB":         unix.MAP_HUGETLB,         /* create a huge page mapping */
		"MAP_LOCKED":          unix.MAP_LOCKED,          /* pages are locked */
		"MAP_NONBLOCK":        unix.MAP_NONBLOCK,        /* do not block on IO */
		"MAP_NORESERVE":       unix.MAP_NORESERVE,       /* don't check for reservations */
		"MAP_POPULATE":        unix.MAP_POPULATE,        /* populate (prefault) pagetables */
		"MAP_STACK":           unix.MAP_STACK,           /* give out an address that is best suited for process/thread stacks */
		"MAP_SYNC":            unix.MAP_SYNC,            /* perform synchronous page faults for the mapping */
		"MAP_UNINITIALIZED":   0x4000000,                /* For anonymous mmap, memory could be uninitialized */
		"MAP_HUGE_16KB":       14 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_64KB":       16 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_512KB":      19 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_1MB":        20 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_2MB":        21 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_8MB":        23 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_16MB":       24 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_32MB":       25 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_256MB":      28 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_512MB":      29 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_1GB":        30 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_2GB":        31 << unix.MAP_HUGE_SHIFT,
		"MAP_HUGE_16GB":       34 << unix.MAP_HUGE_SHIFT,
	}

	// SignalConstants are the supported signals for the kill syscall
	// generate_constants:Signal constants,Signal constants are the supported signals for the kill syscall.
	SignalConstants = map[string]int{
		"SIGHUP":    int(unix.SIGHUP),
		"SIGINT":    int(unix.SIGINT),
		"SIGQUIT":   int(unix.SIGQUIT),
		"SIGILL":    int(unix.SIGILL),
		"SIGTRAP":   int(unix.SIGTRAP),
		"SIGABRT":   int(unix.SIGABRT),
		"SIGIOT":    int(unix.SIGIOT),
		"SIGBUS":    int(unix.SIGBUS),
		"SIGFPE":    int(unix.SIGFPE),
		"SIGKILL":   int(unix.SIGKILL),
		"SIGUSR1":   int(unix.SIGUSR1),
		"SIGSEGV":   int(unix.SIGSEGV),
		"SIGUSR2":   int(unix.SIGUSR2),
		"SIGPIPE":   int(unix.SIGPIPE),
		"SIGALRM":   int(unix.SIGALRM),
		"SIGTERM":   int(unix.SIGTERM),
		"SIGSTKFLT": int(unix.SIGSTKFLT),
		"SIGCHLD":   int(unix.SIGCHLD),
		"SIGCONT":   int(unix.SIGCONT),
		"SIGSTOP":   int(unix.SIGSTOP),
		"SIGTSTP":   int(unix.SIGTSTP),
		"SIGTTIN":   int(unix.SIGTTIN),
		"SIGTTOU":   int(unix.SIGTTOU),
		"SIGURG":    int(unix.SIGURG),
		"SIGXCPU":   int(unix.SIGXCPU),
		"SIGXFSZ":   int(unix.SIGXFSZ),
		"SIGVTALRM": int(unix.SIGVTALRM),
		"SIGPROF":   int(unix.SIGPROF),
		"SIGWINCH":  int(unix.SIGWINCH),
		"SIGIO":     int(unix.SIGIO),
		"SIGPOLL":   int(unix.SIGPOLL),
		"SIGPWR":    int(unix.SIGPWR),
		"SIGSYS":    int(unix.SIGSYS),
	}

	// unlinkFlagsConstants are the supported unlink flags for the unlink syscall
	// generate_constants:Unlink flags,Unlink flags are the supported flags for the unlink syscall.
	unlinkFlagsConstants = map[string]int{
		"AT_REMOVEDIR": unix.AT_REMOVEDIR,
	}

	// addressFamilyConstants are the supported network address families
	// generate_constants:Network Address Family constants,Network Address Family constants are the supported network address families.
	addressFamilyConstants = map[string]uint16{
		"AF_UNSPEC":     unix.AF_UNSPEC,
		"AF_LOCAL":      unix.AF_LOCAL,
		"AF_UNIX":       unix.AF_UNIX,
		"AF_FILE":       unix.AF_FILE,
		"AF_INET":       unix.AF_INET,
		"AF_AX25":       unix.AF_AX25,
		"AF_IPX":        unix.AF_IPX,
		"AF_APPLETALK":  unix.AF_APPLETALK,
		"AF_NETROM":     unix.AF_NETROM,
		"AF_BRIDGE":     unix.AF_BRIDGE,
		"AF_ATMPVC":     unix.AF_ATMPVC,
		"AF_X25":        unix.AF_X25,
		"AF_INET6":      unix.AF_INET6,
		"AF_ROSE":       unix.AF_ROSE,
		"AF_DECnet":     unix.AF_DECnet,
		"AF_NETBEUI":    unix.AF_NETBEUI,
		"AF_SECURITY":   unix.AF_SECURITY,
		"AF_KEY":        unix.AF_KEY,
		"AF_NETLINK":    unix.AF_NETLINK,
		"AF_ROUTE":      unix.AF_ROUTE,
		"AF_PACKET":     unix.AF_PACKET,
		"AF_ASH":        unix.AF_ASH,
		"AF_ECONET":     unix.AF_ECONET,
		"AF_ATMSVC":     unix.AF_ATMSVC,
		"AF_RDS":        unix.AF_RDS,
		"AF_SNA":        unix.AF_SNA,
		"AF_IRDA":       unix.AF_IRDA,
		"AF_PPPOX":      unix.AF_PPPOX,
		"AF_WANPIPE":    unix.AF_WANPIPE,
		"AF_LLC":        unix.AF_LLC,
		"AF_IB":         unix.AF_IB,
		"AF_MPLS":       unix.AF_MPLS,
		"AF_CAN":        unix.AF_CAN,
		"AF_TIPC":       unix.AF_TIPC,
		"AF_BLUETOOTH":  unix.AF_BLUETOOTH,
		"AF_IUCV":       unix.AF_IUCV,
		"AF_RXRPC":      unix.AF_RXRPC,
		"AF_ISDN":       unix.AF_ISDN,
		"AF_PHONET":     unix.AF_PHONET,
		"AF_IEEE802154": unix.AF_IEEE802154,
		"AF_CAIF":       unix.AF_CAIF,
		"AF_ALG":        unix.AF_ALG,
		"AF_NFC":        unix.AF_NFC,
		"AF_VSOCK":      unix.AF_VSOCK,
		"AF_KCM":        unix.AF_KCM,
		"AF_QIPCRTR":    unix.AF_QIPCRTR,
		"AF_SMC":        unix.AF_SMC,
		"AF_XDP":        unix.AF_XDP,
		"AF_MAX":        unix.AF_MAX,
	}

	// vmConstants is the list of protection flags for a virtual memory segment
	// generate_constants:Virtual Memory flags,Virtual Memory flags define the protection of a virtual memory segment.
	vmConstants = map[string]uint64{
		"VM_NONE":         0x0,
		"VM_READ":         0x1,
		"VM_WRITE":        0x2,
		"VM_EXEC":         0x4,
		"VM_SHARED":       0x8,
		"VM_MAYREAD":      0x00000010,
		"VM_MAYWRITE":     0x00000020,
		"VM_MAYEXEC":      0x00000040,
		"VM_MAYSHARE":     0x00000080,
		"VM_GROWSDOWN":    0x00000100, /* general info on the segment */
		"VM_UFFD_MISSING": 0x00000200, /* missing pages tracking */
		"VM_PFNMAP":       0x00000400, /* Page-ranges managed without "struct page", just pure PFN */
		"VM_UFFD_WP":      0x00001000, /* wrprotect pages tracking */
		"VM_LOCKED":       0x00002000,
		"VM_IO":           0x00004000, /* Memory mapped I/O or similar */
		"VM_SEQ_READ":     0x00008000, /* App will access data sequentially */
		"VM_RAND_READ":    0x00010000, /* App will not benefit from clustered reads */
		"VM_DONTCOPY":     0x00020000, /* Do not copy this vma on fork */
		"VM_DONTEXPAND":   0x00040000, /* Cannot expand with mremap() */
		"VM_LOCKONFAULT":  0x00080000, /* Lock the pages covered when they are faulted in */
		"VM_ACCOUNT":      0x00100000, /* Is a VM accounted object */
		"VM_NORESERVE":    0x00200000, /* should the VM suppress accounting */
		"VM_HUGETLB":      0x00400000, /* Huge TLB Page VM */
		"VM_SYNC":         0x00800000, /* Synchronous page faults */
		"VM_ARCH_1":       0x01000000, /* Architecture-specific flag */
		"VM_WIPEONFORK":   0x02000000, /* Wipe VMA contents in child. */
		"VM_DONTDUMP":     0x04000000, /* Do not include in the core dump */
		"VM_SOFTDIRTY":    0x08000000, /* Not soft dirty clean area */
		"VM_MIXEDMAP":     0x10000000, /* Can contain "struct page" and pure PFN pages */
		"VM_HUGEPAGE":     0x20000000, /* MADV_HUGEPAGE marked this vma */
		"VM_NOHUGEPAGE":   0x40000000, /* MADV_NOHUGEPAGE marked this vma */
		"VM_MERGEABLE":    0x80000000, /* KSM may merge identical pages */
	}

	// BPFCmdConstants is the list of BPF commands
	// generate_constants:BPF commands,BPF commands are used to specify a command to a bpf syscall.
	BPFCmdConstants = map[string]BPFCmd{
		"BPF_MAP_CREATE":                  BpfMapCreateCmd,
		"BPF_MAP_LOOKUP_ELEM":             BpfMapLookupElemCmd,
		"BPF_MAP_UPDATE_ELEM":             BpfMapUpdateElemCmd,
		"BPF_MAP_DELETE_ELEM":             BpfMapDeleteElemCmd,
		"BPF_MAP_GET_NEXT_KEY":            BpfMapGetNextKeyCmd,
		"BPF_PROG_LOAD":                   BpfProgLoadCmd,
		"BPF_OBJ_PIN":                     BpfObjPinCmd,
		"BPF_OBJ_GET":                     BpfObjGetCmd,
		"BPF_PROG_ATTACH":                 BpfProgAttachCmd,
		"BPF_PROG_DETACH":                 BpfProgDetachCmd,
		"BPF_PROG_TEST_RUN":               BpfProgTestRunCmd,
		"BPF_PROG_RUN":                    BpfProgTestRunCmd,
		"BPF_PROG_GET_NEXT_ID":            BpfProgGetNextIDCmd,
		"BPF_MAP_GET_NEXT_ID":             BpfMapGetNextIDCmd,
		"BPF_PROG_GET_FD_BY_ID":           BpfProgGetFdByIDCmd,
		"BPF_MAP_GET_FD_BY_ID":            BpfMapGetFdByIDCmd,
		"BPF_OBJ_GET_INFO_BY_FD":          BpfObjGetInfoByFdCmd,
		"BPF_PROG_QUERY":                  BpfProgQueryCmd,
		"BPF_RAW_TRACEPOINT_OPEN":         BpfRawTracepointOpenCmd,
		"BPF_BTF_LOAD":                    BpfBtfLoadCmd,
		"BPF_BTF_GET_FD_BY_ID":            BpfBtfGetFdByIDCmd,
		"BPF_TASK_FD_QUERY":               BpfTaskFdQueryCmd,
		"BPF_MAP_LOOKUP_AND_DELETE_ELEM":  BpfMapLookupAndDeleteElemCmd,
		"BPF_MAP_FREEZE":                  BpfMapFreezeCmd,
		"BPF_BTF_GET_NEXT_ID":             BpfBtfGetNextIDCmd,
		"BPF_MAP_LOOKUP_BATCH":            BpfMapLookupBatchCmd,
		"BPF_MAP_LOOKUP_AND_DELETE_BATCH": BpfMapLookupAndDeleteBatchCmd,
		"BPF_MAP_UPDATE_BATCH":            BpfMapUpdateBatchCmd,
		"BPF_MAP_DELETE_BATCH":            BpfMapDeleteBatchCmd,
		"BPF_LINK_CREATE":                 BpfLinkCreateCmd,
		"BPF_LINK_UPDATE":                 BpfLinkUpdateCmd,
		"BPF_LINK_GET_FD_BY_ID":           BpfLinkGetFdByIDCmd,
		"BPF_LINK_GET_NEXT_ID":            BpfLinkGetNextIDCmd,
		"BPF_ENABLE_STATS":                BpfEnableStatsCmd,
		"BPF_ITER_CREATE":                 BpfIterCreateCmd,
		"BPF_LINK_DETACH":                 BpfLinkDetachCmd,
		"BPF_PROG_BIND_MAP":               BpfProgBindMapCmd,
	}

	// BPFHelperFuncConstants is the list of BPF helper func constants
	// generate_constants:BPF helper functions,BPF helper functions are the supported BPF helper functions.
	BPFHelperFuncConstants = map[string]BPFHelperFunc{
		"BPF_UNSPEC":                         BpfUnspec,
		"BPF_MAP_LOOKUP_ELEM":                BpfMapLookupElem,
		"BPF_MAP_UPDATE_ELEM":                BpfMapUpdateElem,
		"BPF_MAP_DELETE_ELEM":                BpfMapDeleteElem,
		"BPF_PROBE_READ":                     BpfProbeRead,
		"BPF_KTIME_GET_NS":                   BpfKtimeGetNs,
		"BPF_TRACE_PRINTK":                   BpfTracePrintk,
		"BPF_GET_PRANDOM_U32":                BpfGetPrandomU32,
		"BPF_GET_SMP_PROCESSOR_ID":           BpfGetSmpProcessorID,
		"BPF_SKB_STORE_BYTES":                BpfSkbStoreBytes,
		"BPF_L3_CSUM_REPLACE":                BpfL3CsumReplace,
		"BPF_L4_CSUM_REPLACE":                BpfL4CsumReplace,
		"BPF_TAIL_CALL":                      BpfTailCall,
		"BPF_CLONE_REDIRECT":                 BpfCloneRedirect,
		"BPF_GET_CURRENT_PID_TGID":           BpfGetCurrentPidTgid,
		"BPF_GET_CURRENT_UID_GID":            BpfGetCurrentUIDGid,
		"BPF_GET_CURRENT_COMM":               BpfGetCurrentComm,
		"BPF_GET_CGROUP_CLASSID":             BpfGetCgroupClassid,
		"BPF_SKB_VLAN_PUSH":                  BpfSkbVlanPush,
		"BPF_SKB_VLAN_POP":                   BpfSkbVlanPop,
		"BPF_SKB_GET_TUNNEL_KEY":             BpfSkbGetTunnelKey,
		"BPF_SKB_SET_TUNNEL_KEY":             BpfSkbSetTunnelKey,
		"BPF_PERF_EVENT_READ":                BpfPerfEventRead,
		"BPF_REDIRECT":                       BpfRedirect,
		"BPF_GET_ROUTE_REALM":                BpfGetRouteRealm,
		"BPF_PERF_EVENT_OUTPUT":              BpfPerfEventOutput,
		"BPF_SKB_LOAD_BYTES":                 BpfSkbLoadBytes,
		"BPF_GET_STACKID":                    BpfGetStackid,
		"BPF_CSUM_DIFF":                      BpfCsumDiff,
		"BPF_SKB_GET_TUNNEL_OPT":             BpfSkbGetTunnelOpt,
		"BPF_SKB_SET_TUNNEL_OPT":             BpfSkbSetTunnelOpt,
		"BPF_SKB_CHANGE_PROTO":               BpfSkbChangeProto,
		"BPF_SKB_CHANGE_TYPE":                BpfSkbChangeType,
		"BPF_SKB_UNDER_CGROUP":               BpfSkbUnderCgroup,
		"BPF_GET_HASH_RECALC":                BpfGetHashRecalc,
		"BPF_GET_CURRENT_TASK":               BpfGetCurrentTask,
		"BPF_PROBE_WRITE_USER":               BpfProbeWriteUser,
		"BPF_CURRENT_TASK_UNDER_CGROUP":      BpfCurrentTaskUnderCgroup,
		"BPF_SKB_CHANGE_TAIL":                BpfSkbChangeTail,
		"BPF_SKB_PULL_DATA":                  BpfSkbPullData,
		"BPF_CSUM_UPDATE":                    BpfCsumUpdate,
		"BPF_SET_HASH_INVALID":               BpfSetHashInvalid,
		"BPF_GET_NUMA_NODE_ID":               BpfGetNumaNodeID,
		"BPF_SKB_CHANGE_HEAD":                BpfSkbChangeHead,
		"BPF_XDP_ADJUST_HEAD":                BpfXdpAdjustHead,
		"BPF_PROBE_READ_STR":                 BpfProbeReadStr,
		"BPF_GET_SOCKET_COOKIE":              BpfGetSocketCookie,
		"BPF_GET_SOCKET_UID":                 BpfGetSocketUID,
		"BPF_SET_HASH":                       BpfSetHash,
		"BPF_SETSOCKOPT":                     BpfSetsockopt,
		"BPF_SKB_ADJUST_ROOM":                BpfSkbAdjustRoom,
		"BPF_REDIRECT_MAP":                   BpfRedirectMap,
		"BPF_SK_REDIRECT_MAP":                BpfSkRedirectMap,
		"BPF_SOCK_MAP_UPDATE":                BpfSockMapUpdate,
		"BPF_XDP_ADJUST_META":                BpfXdpAdjustMeta,
		"BPF_PERF_EVENT_READ_VALUE":          BpfPerfEventReadValue,
		"BPF_PERF_PROG_READ_VALUE":           BpfPerfProgReadValue,
		"BPF_GETSOCKOPT":                     BpfGetsockopt,
		"BPF_OVERRIDE_RETURN":                BpfOverrideReturn,
		"BPF_SOCK_OPS_CB_FLAGS_SET":          BpfSockOpsCbFlagsSet,
		"BPF_MSG_REDIRECT_MAP":               BpfMsgRedirectMap,
		"BPF_MSG_APPLY_BYTES":                BpfMsgApplyBytes,
		"BPF_MSG_CORK_BYTES":                 BpfMsgCorkBytes,
		"BPF_MSG_PULL_DATA":                  BpfMsgPullData,
		"BPF_BIND":                           BpfBind,
		"BPF_XDP_ADJUST_TAIL":                BpfXdpAdjustTail,
		"BPF_SKB_GET_XFRM_STATE":             BpfSkbGetXfrmState,
		"BPF_GET_STACK":                      BpfGetStack,
		"BPF_SKB_LOAD_BYTES_RELATIVE":        BpfSkbLoadBytesRelative,
		"BPF_FIB_LOOKUP":                     BpfFibLookup,
		"BPF_SOCK_HASH_UPDATE":               BpfSockHashUpdate,
		"BPF_MSG_REDIRECT_HASH":              BpfMsgRedirectHash,
		"BPF_SK_REDIRECT_HASH":               BpfSkRedirectHash,
		"BPF_LWT_PUSH_ENCAP":                 BpfLwtPushEncap,
		"BPF_LWT_SEG6_STORE_BYTES":           BpfLwtSeg6StoreBytes,
		"BPF_LWT_SEG6_ADJUST_SRH":            BpfLwtSeg6AdjustSrh,
		"BPF_LWT_SEG6_ACTION":                BpfLwtSeg6Action,
		"BPF_RC_REPEAT":                      BpfRcRepeat,
		"BPF_RC_KEYDOWN":                     BpfRcKeydown,
		"BPF_SKB_CGROUP_ID":                  BpfSkbCgroupID,
		"BPF_GET_CURRENT_CGROUP_ID":          BpfGetCurrentCgroupID,
		"BPF_GET_LOCAL_STORAGE":              BpfGetLocalStorage,
		"BPF_SK_SELECT_REUSEPORT":            BpfSkSelectReuseport,
		"BPF_SKB_ANCESTOR_CGROUP_ID":         BpfSkbAncestorCgroupID,
		"BPF_SK_LOOKUP_TCP":                  BpfSkLookupTCP,
		"BPF_SK_LOOKUP_UDP":                  BpfSkLookupUDP,
		"BPF_SK_RELEASE":                     BpfSkRelease,
		"BPF_MAP_PUSH_ELEM":                  BpfMapPushElem,
		"BPF_MAP_POP_ELEM":                   BpfMapPopElem,
		"BPF_MAP_PEEK_ELEM":                  BpfMapPeekElem,
		"BPF_MSG_PUSH_DATA":                  BpfMsgPushData,
		"BPF_MSG_POP_DATA":                   BpfMsgPopData,
		"BPF_RC_POINTER_REL":                 BpfRcPointerRel,
		"BPF_SPIN_LOCK":                      BpfSpinLock,
		"BPF_SPIN_UNLOCK":                    BpfSpinUnlock,
		"BPF_SK_FULLSOCK":                    BpfSkFullsock,
		"BPF_TCP_SOCK":                       BpfTCPSock,
		"BPF_SKB_ECN_SET_CE":                 BpfSkbEcnSetCe,
		"BPF_GET_LISTENER_SOCK":              BpfGetListenerSock,
		"BPF_SKC_LOOKUP_TCP":                 BpfSkcLookupTCP,
		"BPF_TCP_CHECK_SYNCOOKIE":            BpfTCPCheckSyncookie,
		"BPF_SYSCTL_GET_NAME":                BpfSysctlGetName,
		"BPF_SYSCTL_GET_CURRENT_VALUE":       BpfSysctlGetCurrentValue,
		"BPF_SYSCTL_GET_NEW_VALUE":           BpfSysctlGetNewValue,
		"BPF_SYSCTL_SET_NEW_VALUE":           BpfSysctlSetNewValue,
		"BPF_STRTOL":                         BpfStrtol,
		"BPF_STRTOUL":                        BpfStrtoul,
		"BPF_SK_STORAGE_GET":                 BpfSkStorageGet,
		"BPF_SK_STORAGE_DELETE":              BpfSkStorageDelete,
		"BPF_SEND_SIGNAL":                    BpfSendSignal,
		"BPF_TCP_GEN_SYNCOOKIE":              BpfTCPGenSyncookie,
		"BPF_SKB_OUTPUT":                     BpfSkbOutput,
		"BPF_PROBE_READ_USER":                BpfProbeReadUser,
		"BPF_PROBE_READ_KERNEL":              BpfProbeReadKernel,
		"BPF_PROBE_READ_USER_STR":            BpfProbeReadUserStr,
		"BPF_PROBE_READ_KERNEL_STR":          BpfProbeReadKernelStr,
		"BPF_TCP_SEND_ACK":                   BpfTCPSendAck,
		"BPF_SEND_SIGNAL_THREAD":             BpfSendSignalThread,
		"BPF_JIFFIES64":                      BpfJiffies64,
		"BPF_READ_BRANCH_RECORDS":            BpfReadBranchRecords,
		"BPF_GET_NS_CURRENT_PID_TGID":        BpfGetNsCurrentPidTgid,
		"BPF_XDP_OUTPUT":                     BpfXdpOutput,
		"BPF_GET_NETNS_COOKIE":               BpfGetNetnsCookie,
		"BPF_GET_CURRENT_ANCESTOR_CGROUP_ID": BpfGetCurrentAncestorCgroupID,
		"BPF_SK_ASSIGN":                      BpfSkAssign,
		"BPF_KTIME_GET_BOOT_NS":              BpfKtimeGetBootNs,
		"BPF_SEQ_PRINTF":                     BpfSeqPrintf,
		"BPF_SEQ_WRITE":                      BpfSeqWrite,
		"BPF_SK_CGROUP_ID":                   BpfSkCgroupID,
		"BPF_SK_ANCESTOR_CGROUP_ID":          BpfSkAncestorCgroupID,
		"BPF_RINGBUF_OUTPUT":                 BpfRingbufOutput,
		"BPF_RINGBUF_RESERVE":                BpfRingbufReserve,
		"BPF_RINGBUF_SUBMIT":                 BpfRingbufSubmit,
		"BPF_RINGBUF_DISCARD":                BpfRingbufDiscard,
		"BPF_RINGBUF_QUERY":                  BpfRingbufQuery,
		"BPF_CSUM_LEVEL":                     BpfCsumLevel,
		"BPF_SKC_TO_TCP6_SOCK":               BpfSkcToTCP6Sock,
		"BPF_SKC_TO_TCP_SOCK":                BpfSkcToTCPSock,
		"BPF_SKC_TO_TCP_TIMEWAIT_SOCK":       BpfSkcToTCPTimewaitSock,
		"BPF_SKC_TO_TCP_REQUEST_SOCK":        BpfSkcToTCPRequestSock,
		"BPF_SKC_TO_UDP6_SOCK":               BpfSkcToUDP6Sock,
		"BPF_GET_TASK_STACK":                 BpfGetTaskStack,
		"BPF_LOAD_HDR_OPT":                   BpfLoadHdrOpt,
		"BPF_STORE_HDR_OPT":                  BpfStoreHdrOpt,
		"BPF_RESERVE_HDR_OPT":                BpfReserveHdrOpt,
		"BPF_INODE_STORAGE_GET":              BpfInodeStorageGet,
		"BPF_INODE_STORAGE_DELETE":           BpfInodeStorageDelete,
		"BPF_D_PATH":                         BpfDPath,
		"BPF_COPY_FROM_USER":                 BpfCopyFromUser,
		"BPF_SNPRINTF_BTF":                   BpfSnprintfBtf,
		"BPF_SEQ_PRINTF_BTF":                 BpfSeqPrintfBtf,
		"BPF_SKB_CGROUP_CLASSID":             BpfSkbCgroupClassid,
		"BPF_REDIRECT_NEIGH":                 BpfRedirectNeigh,
		"BPF_PER_CPU_PTR":                    BpfPerCPUPtr,
		"BPF_THIS_CPU_PTR":                   BpfThisCPUPtr,
		"BPF_REDIRECT_PEER":                  BpfRedirectPeer,
		"BPF_TASK_STORAGE_GET":               BpfTaskStorageGet,
		"BPF_TASK_STORAGE_DELETE":            BpfTaskStorageDelete,
		"BPF_GET_CURRENT_TASK_BTF":           BpfGetCurrentTaskBtf,
		"BPF_BPRM_OPTS_SET":                  BpfBprmOptsSet,
		"BPF_KTIME_GET_COARSE_NS":            BpfKtimeGetCoarseNs,
		"BPF_IMA_INODE_HASH":                 BpfImaInodeHash,
		"BPF_SOCK_FROM_FILE":                 BpfSockFromFile,
		"BPF_CHECK_MTU":                      BpfCheckMtu,
		"BPF_FOR_EACH_MAP_ELEM":              BpfForEachMapElem,
		"BPF_SNPRINTF":                       BpfSnprintf,
	}

	// BPFMapTypeConstants is the list of BPF map type constants
	// generate_constants:BPF map types,BPF map types are the supported eBPF map types.
	BPFMapTypeConstants = map[string]BPFMapType{
		"BPF_MAP_TYPE_UNSPEC":                BpfMapTypeUnspec,
		"BPF_MAP_TYPE_HASH":                  BpfMapTypeHash,
		"BPF_MAP_TYPE_ARRAY":                 BpfMapTypeArray,
		"BPF_MAP_TYPE_PROG_ARRAY":            BpfMapTypeProgArray,
		"BPF_MAP_TYPE_PERF_EVENT_ARRAY":      BpfMapTypePerfEventArray,
		"BPF_MAP_TYPE_PERCPU_HASH":           BpfMapTypePercpuHash,
		"BPF_MAP_TYPE_PERCPU_ARRAY":          BpfMapTypePercpuArray,
		"BPF_MAP_TYPE_STACK_TRACE":           BpfMapTypeStackTrace,
		"BPF_MAP_TYPE_CGROUP_ARRAY":          BpfMapTypeCgroupArray,
		"BPF_MAP_TYPE_LRU_HASH":              BpfMapTypeLruHash,
		"BPF_MAP_TYPE_LRU_PERCPU_HASH":       BpfMapTypeLruPercpuHash,
		"BPF_MAP_TYPE_LPM_TRIE":              BpfMapTypeLpmTrie,
		"BPF_MAP_TYPE_ARRAY_OF_MAPS":         BpfMapTypeArrayOfMaps,
		"BPF_MAP_TYPE_HASH_OF_MAPS":          BpfMapTypeHashOfMaps,
		"BPF_MAP_TYPE_DEVMAP":                BpfMapTypeDevmap,
		"BPF_MAP_TYPE_SOCKMAP":               BpfMapTypeSockmap,
		"BPF_MAP_TYPE_CPUMAP":                BpfMapTypeCPUmap,
		"BPF_MAP_TYPE_XSKMAP":                BpfMapTypeXskmap,
		"BPF_MAP_TYPE_SOCKHASH":              BpfMapTypeSockhash,
		"BPF_MAP_TYPE_CGROUP_STORAGE":        BpfMapTypeCgroupStorage,
		"BPF_MAP_TYPE_REUSEPORT_SOCKARRAY":   BpfMapTypeReuseportSockarray,
		"BPF_MAP_TYPE_PERCPU_CGROUP_STORAGE": BpfMapTypePercpuCgroupStorage,
		"BPF_MAP_TYPE_QUEUE":                 BpfMapTypeQueue,
		"BPF_MAP_TYPE_STACK":                 BpfMapTypeStack,
		"BPF_MAP_TYPE_SK_STORAGE":            BpfMapTypeSkStorage,
		"BPF_MAP_TYPE_DEVMAP_HASH":           BpfMapTypeDevmapHash,
		"BPF_MAP_TYPE_STRUCT_OPS":            BpfMapTypeStructOps,
		"BPF_MAP_TYPE_RINGBUF":               BpfMapTypeRingbuf,
		"BPF_MAP_TYPE_INODE_STORAGE":         BpfMapTypeInodeStorage,
		"BPF_MAP_TYPE_TASK_STORAGE":          BpfMapTypeTaskStorage,
	}

	// BPFProgramTypeConstants is the list of BPF program type constants
	// generate_constants:BPF program types,BPF program types are the supported eBPF program types.
	BPFProgramTypeConstants = map[string]BPFProgramType{
		"BPF_PROG_TYPE_UNSPEC":                  BpfProgTypeUnspec,
		"BPF_PROG_TYPE_SOCKET_FILTER":           BpfProgTypeSocketFilter,
		"BPF_PROG_TYPE_KPROBE":                  BpfProgTypeKprobe,
		"BPF_PROG_TYPE_SCHED_CLS":               BpfProgTypeSchedCls,
		"BPF_PROG_TYPE_SCHED_ACT":               BpfProgTypeSchedAct,
		"BPF_PROG_TYPE_TRACEPOINT":              BpfProgTypeTracepoint,
		"BPF_PROG_TYPE_XDP":                     BpfProgTypeXdp,
		"BPF_PROG_TYPE_PERF_EVENT":              BpfProgTypePerfEvent,
		"BPF_PROG_TYPE_CGROUP_SKB":              BpfProgTypeCgroupSkb,
		"BPF_PROG_TYPE_CGROUP_SOCK":             BpfProgTypeCgroupSock,
		"BPF_PROG_TYPE_LWT_IN":                  BpfProgTypeLwtIn,
		"BPF_PROG_TYPE_LWT_OUT":                 BpfProgTypeLwtOut,
		"BPF_PROG_TYPE_LWT_XMIT":                BpfProgTypeLwtXmit,
		"BPF_PROG_TYPE_SOCK_OPS":                BpfProgTypeSockOps,
		"BPF_PROG_TYPE_SK_SKB":                  BpfProgTypeSkSkb,
		"BPF_PROG_TYPE_CGROUP_DEVICE":           BpfProgTypeCgroupDevice,
		"BPF_PROG_TYPE_SK_MSG":                  BpfProgTypeSkMsg,
		"BPF_PROG_TYPE_RAW_TRACEPOINT":          BpfProgTypeRawTracepoint,
		"BPF_PROG_TYPE_CGROUP_SOCK_ADDR":        BpfProgTypeCgroupSockAddr,
		"BPF_PROG_TYPE_LWT_SEG6LOCAL":           BpfProgTypeLwtSeg6local,
		"BPF_PROG_TYPE_LIRC_MODE2":              BpfProgTypeLircMode2,
		"BPF_PROG_TYPE_SK_REUSEPORT":            BpfProgTypeSkReuseport,
		"BPF_PROG_TYPE_FLOW_DISSECTOR":          BpfProgTypeFlowDissector,
		"BPF_PROG_TYPE_CGROUP_SYSCTL":           BpfProgTypeCgroupSysctl,
		"BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE": BpfProgTypeRawTracepointWritable,
		"BPF_PROG_TYPE_CGROUP_SOCKOPT":          BpfProgTypeCgroupSockopt,
		"BPF_PROG_TYPE_TRACING":                 BpfProgTypeTracing,
		"BPF_PROG_TYPE_STRUCT_OPS":              BpfProgTypeStructOps,
		"BPF_PROG_TYPE_EXT":                     BpfProgTypeExt,
		"BPF_PROG_TYPE_LSM":                     BpfProgTypeLsm,
		"BPF_PROG_TYPE_SK_LOOKUP":               BpfProgTypeSkLookup,
	}

	// BPFAttachTypeConstants is the list of BPF attach type constants
	// generate_constants:BPF attach types,BPF attach types are the supported eBPF program attach types.
	BPFAttachTypeConstants = map[string]BPFAttachType{
		"BPF_CGROUP_INET_INGRESS":      BpfCgroupInetIngress,
		"BPF_CGROUP_INET_EGRESS":       BpfCgroupInetEgress,
		"BPF_CGROUP_INET_SOCK_CREATE":  BpfCgroupInetSockCreate,
		"BPF_CGROUP_SOCK_OPS":          BpfCgroupSockOps,
		"BPF_SK_SKB_STREAM_PARSER":     BpfSkSkbStreamParser,
		"BPF_SK_SKB_STREAM_VERDICT":    BpfSkSkbStreamVerdict,
		"BPF_CGROUP_DEVICE":            BpfCgroupDevice,
		"BPF_SK_MSG_VERDICT":           BpfSkMsgVerdict,
		"BPF_CGROUP_INET4_BIND":        BpfCgroupInet4Bind,
		"BPF_CGROUP_INET6_BIND":        BpfCgroupInet6Bind,
		"BPF_CGROUP_INET4_CONNECT":     BpfCgroupInet4Connect,
		"BPF_CGROUP_INET6_CONNECT":     BpfCgroupInet6Connect,
		"BPF_CGROUP_INET4_POST_BIND":   BpfCgroupInet4PostBind,
		"BPF_CGROUP_INET6_POST_BIND":   BpfCgroupInet6PostBind,
		"BPF_CGROUP_UDP4_SENDMSG":      BpfCgroupUDP4Sendmsg,
		"BPF_CGROUP_UDP6_SENDMSG":      BpfCgroupUDP6Sendmsg,
		"BPF_LIRC_MODE2":               BpfLircMode2,
		"BPF_FLOW_DISSECTOR":           BpfFlowDissector,
		"BPF_CGROUP_SYSCTL":            BpfCgroupSysctl,
		"BPF_CGROUP_UDP4_RECVMSG":      BpfCgroupUDP4Recvmsg,
		"BPF_CGROUP_UDP6_RECVMSG":      BpfCgroupUDP6Recvmsg,
		"BPF_CGROUP_GETSOCKOPT":        BpfCgroupGetsockopt,
		"BPF_CGROUP_SETSOCKOPT":        BpfCgroupSetsockopt,
		"BPF_TRACE_RAW_TP":             BpfTraceRawTp,
		"BPF_TRACE_FENTRY":             BpfTraceFentry,
		"BPF_TRACE_FEXIT":              BpfTraceFexit,
		"BPF_MODIFY_RETURN":            BpfModifyReturn,
		"BPF_LSM_MAC":                  BpfLsmMac,
		"BPF_TRACE_ITER":               BpfTraceIter,
		"BPF_CGROUP_INET4_GETPEERNAME": BpfCgroupInet4Getpeername,
		"BPF_CGROUP_INET6_GETPEERNAME": BpfCgroupInet6Getpeername,
		"BPF_CGROUP_INET4_GETSOCKNAME": BpfCgroupInet4Getsockname,
		"BPF_CGROUP_INET6_GETSOCKNAME": BpfCgroupInet6Getsockname,
		"BPF_XDP_DEVMAP":               BpfXdpDevmap,
		"BPF_CGROUP_INET_SOCK_RELEASE": BpfCgroupInetSockRelease,
		"BPF_XDP_CPUMAP":               BpfXdpCPUmap,
		"BPF_SK_LOOKUP":                BpfSkLookup,
		"BPF_XDP":                      BpfXdp,
		"BPF_SK_SKB_VERDICT":           BpfSkSkbVerdict,
	}

	// PipeBufFlagConstants is the list of pipe buffer flags
	// generate_constants:Pipe buffer flags,Pipe buffer flags are the supported flags for a pipe buffer.
	PipeBufFlagConstants = map[string]PipeBufFlag{
		"PIPE_BUF_FLAG_LRU":       PipeBufFlagLRU,
		"PIPE_BUF_FLAG_ATOMIC":    PipeBufFlagAtomic,
		"PIPE_BUF_FLAG_GIFT":      PipeBufFlagGift,
		"PIPE_BUF_FLAG_PACKET":    PipeBufFlagPacket,
		"PIPE_BUF_FLAG_CAN_MERGE": PipeBufFlagCanMerge,
		"PIPE_BUF_FLAG_WHOLE":     PipeBufFlagWhole,
		"PIPE_BUF_FLAG_LOSS":      PipeBufFlagLoss,
	}
)

func initVMConstants() {
	for k, v := range vmConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
	}

	for k, v := range vmConstants {
		vmStrings[v] = k
	}
}

func initBPFCmdConstants() {
	for k, v := range BPFCmdConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
		bpfCmdStrings[uint32(v)] = k
	}
}

func initBPFHelperFuncConstants() {
	for k, v := range BPFHelperFuncConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
		bpfHelperFuncStrings[uint32(v)] = k
	}
}

func initBPFMapTypeConstants() {
	for k, v := range BPFMapTypeConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
		bpfMapTypeStrings[uint32(v)] = k
	}
}

func initBPFProgramTypeConstants() {
	for k, v := range BPFProgramTypeConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
		bpfProgramTypeStrings[uint32(v)] = k
	}
}

func initBPFAttachTypeConstants() {
	for k, v := range BPFAttachTypeConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
		bpfAttachTypeStrings[uint32(v)] = k
	}
}

func initPipeBufFlagConstants() {
	for k, v := range PipeBufFlagConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
		pipeBufFlagStrings[int(v)] = k
	}
}

func initOpenConstants() {
	for k, v := range openFlagsConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: v}
	}

	for k, v := range openFlagsConstants {
		openFlagsStrings[v] = k
	}
}

func initFileModeConstants() {
	for k, v := range fileModeConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: v}
		fileModeStrings[v] = k
	}
}

func initInodeModeConstants() {
	for k, v := range inodeModeConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: v}
		inodeModeStrings[v] = k
	}
}

func initUnlinkConstanst() {
	for k, v := range unlinkFlagsConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: v}
		unlinkFlagsStrings[v] = k
	}
}

func initKernelCapabilityConstants() {
	for k, v := range KernelCapabilityConstants {
		if bits.UintSize == 64 || v < math.MaxInt32 {
			seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
		}
		kernelCapabilitiesStrings[v] = k
	}
}

func initPtraceConstants() {
	for k, v := range ptraceArchConstants {
		ptraceConstants[k] = v
	}

	for k, v := range ptraceConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
	}

	for k, v := range ptraceConstants {
		ptraceFlagsStrings[v] = k
	}
}

func initProtConstansts() {
	for k, v := range protConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
	}

	for k, v := range protConstants {
		protStrings[v] = k
	}
}

func initMMapFlagsConstants() {
	for k, v := range mmapFlagArchConstants {
		mmapFlagConstants[k] = v
	}

	for k, v := range mmapFlagConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
	}

	for k, v := range mmapFlagConstants {
		mmapFlagStrings[v] = k
	}
}

func initSignalConstants() {
	for k, v := range SignalConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: v}
	}

	for k, v := range SignalConstants {
		signalStrings[v] = k
	}
}

func initBPFMapNamesConstants() {
	seclConstants["CWS_MAP_NAMES"] = &eval.StringArrayEvaluator{Values: bpfMapNames}
}

func bitmaskToStringArray(bitmask int, intToStrMap map[int]string) []string {
	var strs []string
	var result int

	for v, s := range intToStrMap {
		if v == 0 {
			continue
		}

		if bitmask&v == v {
			strs = append(strs, s)
			result |= v
		}
	}

	if result != bitmask {
		strs = append(strs, fmt.Sprintf("%d", bitmask&^result))
	}

	sort.Strings(strs)
	return strs
}

func bitmaskToString(bitmask int, intToStrMap map[int]string) string {
	return strings.Join(bitmaskToStringArray(bitmask, intToStrMap), " | ")
}

func bitmaskU64ToStringArray(bitmask uint64, intToStrMap map[uint64]string) []string {
	var strs []string
	var result uint64

	for v, s := range intToStrMap {
		if v == 0 {
			continue
		}

		if bitmask&v == v {
			strs = append(strs, s)
			result |= v
		}
	}

	if result != bitmask {
		strs = append(strs, fmt.Sprintf("%d", bitmask&^result))
	}

	sort.Strings(strs)
	return strs
}

func bitmaskU64ToString(bitmask uint64, intToStrMap map[uint64]string) string {
	return strings.Join(bitmaskU64ToStringArray(bitmask, intToStrMap), " | ")
}

// OpenFlags represents an open flags bitmask value
type OpenFlags int

func (f OpenFlags) String() string {
	return strings.Join(f.StringArray(), " | ")
}

// StringArray returns the open flags as an array of strings
func (f OpenFlags) StringArray() []string {
	// open flags are actually composed of 2 sets of flags
	// the lowest 2 bits manage the read/write access modes
	readWriteBits := int(f) & 0b11
	// the other bits manage the general purpose flags (like O_CLOEXEC, or O_TRUNC)
	flagsBits := int(f) & ^0b11

	// in order to default to O_RDONLY even if other bits are set we convert
	// both bitmask separately
	readWrite := bitmaskToStringArray(readWriteBits, openFlagsStrings)
	flags := bitmaskToStringArray(flagsBits, openFlagsStrings)

	if len(readWrite) == 0 {
		readWrite = []string{openFlagsStrings[syscall.O_RDONLY]}
	}

	if len(flags) == 0 {
		return readWrite
	}

	return append(readWrite, flags...)
}

// FileMode represents a file mode bitmask value
type FileMode int

func (m FileMode) String() string {
	return bitmaskToString(int(m), fileModeStrings)
}

// InodeMode represents an inode mode bitmask value
type InodeMode int

func (m InodeMode) String() string {
	return bitmaskToString(int(m), inodeModeStrings)
}

// UnlinkFlags represents an unlink flags bitmask value
type UnlinkFlags int

func (f UnlinkFlags) String() string {
	return bitmaskToString(int(f), unlinkFlagsStrings)
}

// StringArray returns the unlink flags as an array of strings
func (f UnlinkFlags) StringArray() []string {
	return bitmaskToStringArray(int(f), unlinkFlagsStrings)
}

// KernelCapability represents a kernel capability bitmask value
type KernelCapability uint64

func (kc KernelCapability) String() string {
	return bitmaskU64ToString(uint64(kc), kernelCapabilitiesStrings)
}

// StringArray returns the kernel capabilities as an array of strings
func (kc KernelCapability) StringArray() []string {
	if kc == 0 {
		return nil
	}
	if value, ok := capsStringArrayCache.Get(kc); ok {
		return value
	}
	computed := bitmaskU64ToStringArray(uint64(kc), kernelCapabilitiesStrings)
	capsStringArrayCache.Add(kc, computed)
	return computed
}

// BPFCmd represents a BPF command
type BPFCmd uint64

func (cmd BPFCmd) String() string {
	return bpfCmdStrings[uint32(cmd)]
}

const (
	// BpfMapCreateCmd command
	BpfMapCreateCmd BPFCmd = iota
	// BpfMapLookupElemCmd command
	BpfMapLookupElemCmd
	// BpfMapUpdateElemCmd command
	BpfMapUpdateElemCmd
	// BpfMapDeleteElemCmd command
	BpfMapDeleteElemCmd
	// BpfMapGetNextKeyCmd command
	BpfMapGetNextKeyCmd
	// BpfProgLoadCmd command
	BpfProgLoadCmd
	// BpfObjPinCmd command
	BpfObjPinCmd
	// BpfObjGetCmd command
	BpfObjGetCmd
	// BpfProgAttachCmd command
	BpfProgAttachCmd
	// BpfProgDetachCmd command
	BpfProgDetachCmd
	// BpfProgTestRunCmd command
	BpfProgTestRunCmd
	// BpfProgGetNextIDCmd command
	BpfProgGetNextIDCmd
	// BpfMapGetNextIDCmd command
	BpfMapGetNextIDCmd
	// BpfProgGetFdByIDCmd command
	BpfProgGetFdByIDCmd
	// BpfMapGetFdByIDCmd command
	BpfMapGetFdByIDCmd
	// BpfObjGetInfoByFdCmd command
	BpfObjGetInfoByFdCmd
	// BpfProgQueryCmd command
	BpfProgQueryCmd
	// BpfRawTracepointOpenCmd command
	BpfRawTracepointOpenCmd
	// BpfBtfLoadCmd command
	BpfBtfLoadCmd
	// BpfBtfGetFdByIDCmd command
	BpfBtfGetFdByIDCmd
	// BpfTaskFdQueryCmd command
	BpfTaskFdQueryCmd
	// BpfMapLookupAndDeleteElemCmd command
	BpfMapLookupAndDeleteElemCmd
	// BpfMapFreezeCmd command
	BpfMapFreezeCmd
	// BpfBtfGetNextIDCmd command
	BpfBtfGetNextIDCmd
	// BpfMapLookupBatchCmd command
	BpfMapLookupBatchCmd
	// BpfMapLookupAndDeleteBatchCmd command
	BpfMapLookupAndDeleteBatchCmd
	// BpfMapUpdateBatchCmd command
	BpfMapUpdateBatchCmd
	// BpfMapDeleteBatchCmd command
	BpfMapDeleteBatchCmd
	// BpfLinkCreateCmd command
	BpfLinkCreateCmd
	// BpfLinkUpdateCmd command
	BpfLinkUpdateCmd
	// BpfLinkGetFdByIDCmd command
	BpfLinkGetFdByIDCmd
	// BpfLinkGetNextIDCmd command
	BpfLinkGetNextIDCmd
	// BpfEnableStatsCmd command
	BpfEnableStatsCmd
	// BpfIterCreateCmd command
	BpfIterCreateCmd
	// BpfLinkDetachCmd command
	BpfLinkDetachCmd
	// BpfProgBindMapCmd command
	BpfProgBindMapCmd
)

// BPFHelperFunc represents a BPF helper function
type BPFHelperFunc uint32

func (f BPFHelperFunc) String() string {
	return bpfHelperFuncStrings[uint32(f)]
}

// StringifyHelpersList returns a string list representation of a list of helpers
func StringifyHelpersList(input []uint32) []string {
	helpers := make([]string, len(input))
	for i, helper := range input {
		helpers[i] = BPFHelperFunc(helper).String()
	}
	return helpers
}

const (
	// BpfUnspec helper function
	BpfUnspec BPFHelperFunc = iota
	// BpfMapLookupElem helper function
	BpfMapLookupElem
	// BpfMapUpdateElem helper function
	BpfMapUpdateElem
	// BpfMapDeleteElem helper function
	BpfMapDeleteElem
	// BpfProbeRead helper function
	BpfProbeRead
	// BpfKtimeGetNs helper function
	BpfKtimeGetNs
	// BpfTracePrintk helper function
	BpfTracePrintk
	// BpfGetPrandomU32 helper function
	BpfGetPrandomU32
	// BpfGetSmpProcessorID helper function
	BpfGetSmpProcessorID
	// BpfSkbStoreBytes helper function
	BpfSkbStoreBytes
	// BpfL3CsumReplace helper function
	BpfL3CsumReplace
	// BpfL4CsumReplace helper function
	BpfL4CsumReplace
	// BpfTailCall helper function
	BpfTailCall
	// BpfCloneRedirect helper function
	BpfCloneRedirect
	// BpfGetCurrentPidTgid helper function
	BpfGetCurrentPidTgid
	// BpfGetCurrentUIDGid helper function
	BpfGetCurrentUIDGid
	// BpfGetCurrentComm helper function
	BpfGetCurrentComm
	// BpfGetCgroupClassid helper function
	BpfGetCgroupClassid
	// BpfSkbVlanPush helper function
	BpfSkbVlanPush
	// BpfSkbVlanPop helper function
	BpfSkbVlanPop
	// BpfSkbGetTunnelKey helper function
	BpfSkbGetTunnelKey
	// BpfSkbSetTunnelKey helper function
	BpfSkbSetTunnelKey
	// BpfPerfEventRead helper function
	BpfPerfEventRead
	// BpfRedirect helper function
	BpfRedirect
	// BpfGetRouteRealm helper function
	BpfGetRouteRealm
	// BpfPerfEventOutput helper function
	BpfPerfEventOutput
	// BpfSkbLoadBytes helper function
	BpfSkbLoadBytes
	// BpfGetStackid helper function
	BpfGetStackid
	// BpfCsumDiff helper function
	BpfCsumDiff
	// BpfSkbGetTunnelOpt helper function
	BpfSkbGetTunnelOpt
	// BpfSkbSetTunnelOpt helper function
	BpfSkbSetTunnelOpt
	// BpfSkbChangeProto helper function
	BpfSkbChangeProto
	// BpfSkbChangeType helper function
	BpfSkbChangeType
	// BpfSkbUnderCgroup helper function
	BpfSkbUnderCgroup
	// BpfGetHashRecalc helper function
	BpfGetHashRecalc
	// BpfGetCurrentTask helper function
	BpfGetCurrentTask
	// BpfProbeWriteUser helper function
	BpfProbeWriteUser
	// BpfCurrentTaskUnderCgroup helper function
	BpfCurrentTaskUnderCgroup
	// BpfSkbChangeTail helper function
	BpfSkbChangeTail
	// BpfSkbPullData helper function
	BpfSkbPullData
	// BpfCsumUpdate helper function
	BpfCsumUpdate
	// BpfSetHashInvalid helper function
	BpfSetHashInvalid
	// BpfGetNumaNodeID helper function
	BpfGetNumaNodeID
	// BpfSkbChangeHead helper function
	BpfSkbChangeHead
	// BpfXdpAdjustHead helper function
	BpfXdpAdjustHead
	// BpfProbeReadStr helper function
	BpfProbeReadStr
	// BpfGetSocketCookie helper function
	BpfGetSocketCookie
	// BpfGetSocketUID helper function
	BpfGetSocketUID
	// BpfSetHash helper function
	BpfSetHash
	// BpfSetsockopt helper function
	BpfSetsockopt
	// BpfSkbAdjustRoom helper function
	BpfSkbAdjustRoom
	// BpfRedirectMap helper function
	BpfRedirectMap
	// BpfSkRedirectMap helper function
	BpfSkRedirectMap
	// BpfSockMapUpdate helper function
	BpfSockMapUpdate
	// BpfXdpAdjustMeta helper function
	BpfXdpAdjustMeta
	// BpfPerfEventReadValue helper function
	BpfPerfEventReadValue
	// BpfPerfProgReadValue helper function
	BpfPerfProgReadValue
	// BpfGetsockopt helper function
	BpfGetsockopt
	// BpfOverrideReturn helper function
	BpfOverrideReturn
	// BpfSockOpsCbFlagsSet helper function
	BpfSockOpsCbFlagsSet
	// BpfMsgRedirectMap helper function
	BpfMsgRedirectMap
	// BpfMsgApplyBytes helper function
	BpfMsgApplyBytes
	// BpfMsgCorkBytes helper function
	BpfMsgCorkBytes
	// BpfMsgPullData helper function
	BpfMsgPullData
	// BpfBind helper function
	BpfBind
	// BpfXdpAdjustTail helper function
	BpfXdpAdjustTail
	// BpfSkbGetXfrmState helper function
	BpfSkbGetXfrmState
	// BpfGetStack helper function
	BpfGetStack
	// BpfSkbLoadBytesRelative helper function
	BpfSkbLoadBytesRelative
	// BpfFibLookup helper function
	BpfFibLookup
	// BpfSockHashUpdate helper function
	BpfSockHashUpdate
	// BpfMsgRedirectHash helper function
	BpfMsgRedirectHash
	// BpfSkRedirectHash helper function
	BpfSkRedirectHash
	// BpfLwtPushEncap helper function
	BpfLwtPushEncap
	// BpfLwtSeg6StoreBytes helper function
	BpfLwtSeg6StoreBytes
	// BpfLwtSeg6AdjustSrh helper function
	BpfLwtSeg6AdjustSrh
	// BpfLwtSeg6Action helper function
	BpfLwtSeg6Action
	// BpfRcRepeat helper function
	BpfRcRepeat
	// BpfRcKeydown helper function
	BpfRcKeydown
	// BpfSkbCgroupID helper function
	BpfSkbCgroupID
	// BpfGetCurrentCgroupID helper function
	BpfGetCurrentCgroupID
	// BpfGetLocalStorage helper function
	BpfGetLocalStorage
	// BpfSkSelectReuseport helper function
	BpfSkSelectReuseport
	// BpfSkbAncestorCgroupID helper function
	BpfSkbAncestorCgroupID
	// BpfSkLookupTCP helper function
	BpfSkLookupTCP
	// BpfSkLookupUDP helper function
	BpfSkLookupUDP
	// BpfSkRelease helper function
	BpfSkRelease
	// BpfMapPushElem helper function
	BpfMapPushElem
	// BpfMapPopElem helper function
	BpfMapPopElem
	// BpfMapPeekElem helper function
	BpfMapPeekElem
	// BpfMsgPushData helper function
	BpfMsgPushData
	// BpfMsgPopData helper function
	BpfMsgPopData
	// BpfRcPointerRel helper function
	BpfRcPointerRel
	// BpfSpinLock helper function
	BpfSpinLock
	// BpfSpinUnlock helper function
	BpfSpinUnlock
	// BpfSkFullsock helper function
	BpfSkFullsock
	// BpfTCPSock helper function
	BpfTCPSock
	// BpfSkbEcnSetCe helper function
	BpfSkbEcnSetCe
	// BpfGetListenerSock helper function
	BpfGetListenerSock
	// BpfSkcLookupTCP helper function
	BpfSkcLookupTCP
	// BpfTCPCheckSyncookie helper function
	BpfTCPCheckSyncookie
	// BpfSysctlGetName helper function
	BpfSysctlGetName
	// BpfSysctlGetCurrentValue helper function
	BpfSysctlGetCurrentValue
	// BpfSysctlGetNewValue helper function
	BpfSysctlGetNewValue
	// BpfSysctlSetNewValue helper function
	BpfSysctlSetNewValue
	// BpfStrtol helper function
	BpfStrtol
	// BpfStrtoul helper function
	BpfStrtoul
	// BpfSkStorageGet helper function
	BpfSkStorageGet
	// BpfSkStorageDelete helper function
	BpfSkStorageDelete
	// BpfSendSignal helper function
	BpfSendSignal
	// BpfTCPGenSyncookie helper function
	BpfTCPGenSyncookie
	// BpfSkbOutput helper function
	BpfSkbOutput
	// BpfProbeReadUser helper function
	BpfProbeReadUser
	// BpfProbeReadKernel helper function
	BpfProbeReadKernel
	// BpfProbeReadUserStr helper function
	BpfProbeReadUserStr
	// BpfProbeReadKernelStr helper function
	BpfProbeReadKernelStr
	// BpfTCPSendAck helper function
	BpfTCPSendAck
	// BpfSendSignalThread helper function
	BpfSendSignalThread
	// BpfJiffies64 helper function
	BpfJiffies64
	// BpfReadBranchRecords helper function
	BpfReadBranchRecords
	// BpfGetNsCurrentPidTgid helper function
	BpfGetNsCurrentPidTgid
	// BpfXdpOutput helper function
	BpfXdpOutput
	// BpfGetNetnsCookie helper function
	BpfGetNetnsCookie
	// BpfGetCurrentAncestorCgroupID helper function
	BpfGetCurrentAncestorCgroupID
	// BpfSkAssign helper function
	BpfSkAssign
	// BpfKtimeGetBootNs helper function
	BpfKtimeGetBootNs
	// BpfSeqPrintf helper function
	BpfSeqPrintf
	// BpfSeqWrite helper function
	BpfSeqWrite
	// BpfSkCgroupID helper function
	BpfSkCgroupID
	// BpfSkAncestorCgroupID helper function
	BpfSkAncestorCgroupID
	// BpfRingbufOutput helper function
	BpfRingbufOutput
	// BpfRingbufReserve helper function
	BpfRingbufReserve
	// BpfRingbufSubmit helper function
	BpfRingbufSubmit
	// BpfRingbufDiscard helper function
	BpfRingbufDiscard
	// BpfRingbufQuery helper function
	BpfRingbufQuery
	// BpfCsumLevel helper function
	BpfCsumLevel
	// BpfSkcToTCP6Sock helper function
	BpfSkcToTCP6Sock
	// BpfSkcToTCPSock helper function
	BpfSkcToTCPSock
	// BpfSkcToTCPTimewaitSock helper function
	BpfSkcToTCPTimewaitSock
	// BpfSkcToTCPRequestSock helper function
	BpfSkcToTCPRequestSock
	// BpfSkcToUDP6Sock helper function
	BpfSkcToUDP6Sock
	// BpfGetTaskStack helper function
	BpfGetTaskStack
	// BpfLoadHdrOpt helper function
	BpfLoadHdrOpt
	// BpfStoreHdrOpt helper function
	BpfStoreHdrOpt
	// BpfReserveHdrOpt helper function
	BpfReserveHdrOpt
	// BpfInodeStorageGet helper function
	BpfInodeStorageGet
	// BpfInodeStorageDelete helper function
	BpfInodeStorageDelete
	// BpfDPath helper function
	BpfDPath
	// BpfCopyFromUser helper function
	BpfCopyFromUser
	// BpfSnprintfBtf helper function
	BpfSnprintfBtf
	// BpfSeqPrintfBtf helper function
	BpfSeqPrintfBtf
	// BpfSkbCgroupClassid helper function
	BpfSkbCgroupClassid
	// BpfRedirectNeigh helper function
	BpfRedirectNeigh
	// BpfPerCPUPtr helper function
	BpfPerCPUPtr
	// BpfThisCPUPtr helper function
	BpfThisCPUPtr
	// BpfRedirectPeer helper function
	BpfRedirectPeer
	// BpfTaskStorageGet helper function
	BpfTaskStorageGet
	// BpfTaskStorageDelete helper function
	BpfTaskStorageDelete
	// BpfGetCurrentTaskBtf helper function
	BpfGetCurrentTaskBtf
	// BpfBprmOptsSet helper function
	BpfBprmOptsSet
	// BpfKtimeGetCoarseNs helper function
	BpfKtimeGetCoarseNs
	// BpfImaInodeHash helper function
	BpfImaInodeHash
	// BpfSockFromFile helper function
	BpfSockFromFile
	// BpfCheckMtu helper function
	BpfCheckMtu
	// BpfForEachMapElem helper function
	BpfForEachMapElem
	// BpfSnprintf helper function
	BpfSnprintf
)

// BPFMapType is used to define map type constants
type BPFMapType uint32

func (t BPFMapType) String() string {
	return bpfMapTypeStrings[uint32(t)]
}

const (
	// BpfMapTypeUnspec map type
	BpfMapTypeUnspec BPFMapType = iota
	// BpfMapTypeHash map type
	BpfMapTypeHash
	// BpfMapTypeArray map type
	BpfMapTypeArray
	// BpfMapTypeProgArray map type
	BpfMapTypeProgArray
	// BpfMapTypePerfEventArray map type
	BpfMapTypePerfEventArray
	// BpfMapTypePercpuHash map type
	BpfMapTypePercpuHash
	// BpfMapTypePercpuArray map type
	BpfMapTypePercpuArray
	// BpfMapTypeStackTrace map type
	BpfMapTypeStackTrace
	// BpfMapTypeCgroupArray map type
	BpfMapTypeCgroupArray
	// BpfMapTypeLruHash map type
	BpfMapTypeLruHash
	// BpfMapTypeLruPercpuHash map type
	BpfMapTypeLruPercpuHash
	// BpfMapTypeLpmTrie map type
	BpfMapTypeLpmTrie
	// BpfMapTypeArrayOfMaps map type
	BpfMapTypeArrayOfMaps
	// BpfMapTypeHashOfMaps map type
	BpfMapTypeHashOfMaps
	// BpfMapTypeDevmap map type
	BpfMapTypeDevmap
	// BpfMapTypeSockmap map type
	BpfMapTypeSockmap
	// BpfMapTypeCPUmap map type
	BpfMapTypeCPUmap
	// BpfMapTypeXskmap map type
	BpfMapTypeXskmap
	// BpfMapTypeSockhash map type
	BpfMapTypeSockhash
	// BpfMapTypeCgroupStorage map type
	BpfMapTypeCgroupStorage
	// BpfMapTypeReuseportSockarray map type
	BpfMapTypeReuseportSockarray
	// BpfMapTypePercpuCgroupStorage map type
	BpfMapTypePercpuCgroupStorage
	// BpfMapTypeQueue map type
	BpfMapTypeQueue
	// BpfMapTypeStack map type
	BpfMapTypeStack
	// BpfMapTypeSkStorage map type
	BpfMapTypeSkStorage
	// BpfMapTypeDevmapHash map type
	BpfMapTypeDevmapHash
	// BpfMapTypeStructOps map type
	BpfMapTypeStructOps
	// BpfMapTypeRingbuf map type
	BpfMapTypeRingbuf
	// BpfMapTypeInodeStorage map type
	BpfMapTypeInodeStorage
	// BpfMapTypeTaskStorage map type
	BpfMapTypeTaskStorage
)

// BPFProgramType is used to define program type constants
type BPFProgramType uint32

func (t BPFProgramType) String() string {
	return bpfProgramTypeStrings[uint32(t)]
}

const (
	// BpfProgTypeUnspec program type
	BpfProgTypeUnspec BPFProgramType = iota
	// BpfProgTypeSocketFilter program type
	BpfProgTypeSocketFilter
	// BpfProgTypeKprobe program type
	BpfProgTypeKprobe
	// BpfProgTypeSchedCls program type
	BpfProgTypeSchedCls
	// BpfProgTypeSchedAct program type
	BpfProgTypeSchedAct
	// BpfProgTypeTracepoint program type
	BpfProgTypeTracepoint
	// BpfProgTypeXdp program type
	BpfProgTypeXdp
	// BpfProgTypePerfEvent program type
	BpfProgTypePerfEvent
	// BpfProgTypeCgroupSkb program type
	BpfProgTypeCgroupSkb
	// BpfProgTypeCgroupSock program type
	BpfProgTypeCgroupSock
	// BpfProgTypeLwtIn program type
	BpfProgTypeLwtIn
	// BpfProgTypeLwtOut program type
	BpfProgTypeLwtOut
	// BpfProgTypeLwtXmit program type
	BpfProgTypeLwtXmit
	// BpfProgTypeSockOps program type
	BpfProgTypeSockOps
	// BpfProgTypeSkSkb program type
	BpfProgTypeSkSkb
	// BpfProgTypeCgroupDevice program type
	BpfProgTypeCgroupDevice
	// BpfProgTypeSkMsg program type
	BpfProgTypeSkMsg
	// BpfProgTypeRawTracepoint program type
	BpfProgTypeRawTracepoint
	// BpfProgTypeCgroupSockAddr program type
	BpfProgTypeCgroupSockAddr
	// BpfProgTypeLwtSeg6local program type
	BpfProgTypeLwtSeg6local
	// BpfProgTypeLircMode2 program type
	BpfProgTypeLircMode2
	// BpfProgTypeSkReuseport program type
	BpfProgTypeSkReuseport
	// BpfProgTypeFlowDissector program type
	BpfProgTypeFlowDissector
	// BpfProgTypeCgroupSysctl program type
	BpfProgTypeCgroupSysctl
	// BpfProgTypeRawTracepointWritable program type
	BpfProgTypeRawTracepointWritable
	// BpfProgTypeCgroupSockopt program type
	BpfProgTypeCgroupSockopt
	// BpfProgTypeTracing program type
	BpfProgTypeTracing
	// BpfProgTypeStructOps program type
	BpfProgTypeStructOps
	// BpfProgTypeExt program type
	BpfProgTypeExt
	// BpfProgTypeLsm program type
	BpfProgTypeLsm
	// BpfProgTypeSkLookup program type
	BpfProgTypeSkLookup
)

// BPFAttachType is used to define attach type constants
type BPFAttachType uint32

func (t BPFAttachType) String() string {
	return bpfAttachTypeStrings[uint32(t)]
}

const (
	// BpfCgroupInetIngress attach type
	BpfCgroupInetIngress BPFAttachType = iota + 1
	// BpfCgroupInetEgress attach type
	BpfCgroupInetEgress
	// BpfCgroupInetSockCreate attach type
	BpfCgroupInetSockCreate
	// BpfCgroupSockOps attach type
	BpfCgroupSockOps
	// BpfSkSkbStreamParser attach type
	BpfSkSkbStreamParser
	// BpfSkSkbStreamVerdict attach type
	BpfSkSkbStreamVerdict
	// BpfCgroupDevice attach type
	BpfCgroupDevice
	// BpfSkMsgVerdict attach type
	BpfSkMsgVerdict
	// BpfCgroupInet4Bind attach type
	BpfCgroupInet4Bind
	// BpfCgroupInet6Bind attach type
	BpfCgroupInet6Bind
	// BpfCgroupInet4Connect attach type
	BpfCgroupInet4Connect
	// BpfCgroupInet6Connect attach type
	BpfCgroupInet6Connect
	// BpfCgroupInet4PostBind attach type
	BpfCgroupInet4PostBind
	// BpfCgroupInet6PostBind attach type
	BpfCgroupInet6PostBind
	// BpfCgroupUDP4Sendmsg attach type
	BpfCgroupUDP4Sendmsg
	// BpfCgroupUDP6Sendmsg attach type
	BpfCgroupUDP6Sendmsg
	// BpfLircMode2 attach type
	BpfLircMode2
	// BpfFlowDissector attach type
	BpfFlowDissector
	// BpfCgroupSysctl attach type
	BpfCgroupSysctl
	// BpfCgroupUDP4Recvmsg attach type
	BpfCgroupUDP4Recvmsg
	// BpfCgroupUDP6Recvmsg attach type
	BpfCgroupUDP6Recvmsg
	// BpfCgroupGetsockopt attach type
	BpfCgroupGetsockopt
	// BpfCgroupSetsockopt attach type
	BpfCgroupSetsockopt
	// BpfTraceRawTp attach type
	BpfTraceRawTp
	// BpfTraceFentry attach type
	BpfTraceFentry
	// BpfTraceFexit attach type
	BpfTraceFexit
	// BpfModifyReturn attach type
	BpfModifyReturn
	// BpfLsmMac attach type
	BpfLsmMac
	// BpfTraceIter attach type
	BpfTraceIter
	// BpfCgroupInet4Getpeername attach type
	BpfCgroupInet4Getpeername
	// BpfCgroupInet6Getpeername attach type
	BpfCgroupInet6Getpeername
	// BpfCgroupInet4Getsockname attach type
	BpfCgroupInet4Getsockname
	// BpfCgroupInet6Getsockname attach type
	BpfCgroupInet6Getsockname
	// BpfXdpDevmap attach type
	BpfXdpDevmap
	// BpfCgroupInetSockRelease attach type
	BpfCgroupInetSockRelease
	// BpfXdpCPUmap attach type
	BpfXdpCPUmap
	// BpfSkLookup attach type
	BpfSkLookup
	// BpfXdp attach type
	BpfXdp
	// BpfSkSkbVerdict attach type
	BpfSkSkbVerdict
)

var capsStringArrayCache *lru.Cache[KernelCapability, []string]

func init() {
	capsStringArrayCache, _ = lru.New[KernelCapability, []string](4)
}

// PTraceRequest represents a ptrace request value
type PTraceRequest uint32

func (f PTraceRequest) String() string {
	for val, str := range ptraceFlagsStrings {
		if val == uint32(f) {
			return str
		}
	}
	return fmt.Sprintf("%d", f)
}

// VMFlag represents a VM_* bitmask value
type VMFlag uint64

func (vmf VMFlag) String() string {
	return bitmaskU64ToString(uint64(vmf), vmStrings)
}

// Protection represents a virtual memory protection bitmask value
type Protection uint64

func (p Protection) String() string {
	return bitmaskU64ToString(uint64(p), protStrings)
}

// MMapFlag represents a mmap flag value
type MMapFlag uint64

func (mmf MMapFlag) String() string {
	return bitmaskU64ToString(uint64(mmf), mmapFlagStrings)
}

// PipeBufFlag represents a pipe buffer flag
type PipeBufFlag int

func (pbf PipeBufFlag) String() string {
	return bitmaskToString(int(pbf), pipeBufFlagStrings)
}

const (
	// PipeBufFlagLRU pipe buffer flag
	PipeBufFlagLRU PipeBufFlag = 0x1 /* page is on the LRU */
	// PipeBufFlagAtomic pipe buffer flag
	PipeBufFlagAtomic PipeBufFlag = 0x2 /* was atomically mapped */
	// PipeBufFlagGift pipe buffer flag
	PipeBufFlagGift PipeBufFlag = 0x4 /* page is a gift */
	// PipeBufFlagPacket pipe buffer flag
	PipeBufFlagPacket PipeBufFlag = 0x8 /* read() as a packet */
	// PipeBufFlagCanMerge pipe buffer flag
	PipeBufFlagCanMerge PipeBufFlag = 0x10 /* can merge buffers */
	// PipeBufFlagWhole pipe buffer flag
	PipeBufFlagWhole PipeBufFlag = 0x20 /* read() must return entire buffer or error */
	// PipeBufFlagLoss pipe buffer flag
	PipeBufFlagLoss PipeBufFlag = 0x40 /* Message loss happened after this buffer */
)

// Signal represents a type of unix signal (ie, SIGKILL, SIGSTOP etc)
type Signal int

func (sig Signal) String() string {
	return signalStrings[int(sig)]
}

var (
	openFlagsStrings          = map[int]string{}
	fileModeStrings           = map[int]string{}
	inodeModeStrings          = map[int]string{}
	unlinkFlagsStrings        = map[int]string{}
	kernelCapabilitiesStrings = map[uint64]string{}
	bpfCmdStrings             = map[uint32]string{}
	bpfHelperFuncStrings      = map[uint32]string{}
	bpfMapTypeStrings         = map[uint32]string{}
	bpfProgramTypeStrings     = map[uint32]string{}
	bpfAttachTypeStrings      = map[uint32]string{}
	ptraceFlagsStrings        = map[uint32]string{}
	vmStrings                 = map[uint64]string{}
	protStrings               = map[uint64]string{}
	mmapFlagStrings           = map[uint64]string{}
	signalStrings             = map[int]string{}
	pipeBufFlagStrings        = map[int]string{}
)
