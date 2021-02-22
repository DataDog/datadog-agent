// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package model

import (
	"fmt"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// MaxSegmentLength defines the maximum length of each segment of a path
const MaxSegmentLength = 127

// MaxPathDepth defines the maximum depth of a path
const MaxPathDepth = 15

var (
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
		//"O_LARGEFILE": syscall.O_LARGEFILE, golang defines this as 0
		"O_NDELAY":   syscall.O_NDELAY,
		"O_NOATIME":  syscall.O_NOATIME,
		"O_NOCTTY":   syscall.O_NOCTTY,
		"O_NOFOLLOW": syscall.O_NOFOLLOW,
		"O_NONBLOCK": syscall.O_NONBLOCK,
		"O_RSYNC":    syscall.O_RSYNC,
	}

	chmodModeConstants = map[string]int{
		//"S_IEXEC":  syscall.S_IEXEC, deprecated
		"S_IFBLK":  syscall.S_IFBLK,
		"S_IFCHR":  syscall.S_IFCHR,
		"S_IFDIR":  syscall.S_IFDIR,
		"S_IFIFO":  syscall.S_IFIFO,
		"S_IFLNK":  syscall.S_IFLNK,
		"S_IFMT":   syscall.S_IFMT,
		"S_IFREG":  syscall.S_IFREG,
		"S_IFSOCK": syscall.S_IFSOCK,
		//"S_IREAD":  syscall.S_IREAD, deprecated
		"S_IRGRP": syscall.S_IRGRP,
		"S_IROTH": syscall.S_IROTH,
		"S_IRUSR": syscall.S_IRUSR,
		"S_IRWXG": syscall.S_IRWXG,
		"S_IRWXO": syscall.S_IRWXO,
		"S_IRWXU": syscall.S_IRWXU,
		"S_ISGID": syscall.S_ISGID,
		"S_ISUID": syscall.S_ISUID,
		"S_ISVTX": syscall.S_ISVTX,
		"S_IWGRP": syscall.S_IWGRP,
		"S_IWOTH": syscall.S_IWOTH,
		//"S_IWRITE": syscall.S_IWRITE, deprecated
		"S_IWUSR": syscall.S_IWUSR,
		"S_IXGRP": syscall.S_IXGRP,
		"S_IXOTH": syscall.S_IXOTH,
		"S_IXUSR": syscall.S_IXUSR,
	}

	kernelCapabilityConstants = map[string]int{
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
		"CAP_LAST_CAP":           1 << unix.CAP_LAST_CAP,
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

	unlinkFlagsConstants = map[string]int{
		"AT_REMOVEDIR": unix.AT_REMOVEDIR,
	}

	// SECLConstants are constants available in runtime security agent rules
	SECLConstants = map[string]interface{}{
		// boolean
		"true":  &eval.BoolEvaluator{Value: true},
		"false": &eval.BoolEvaluator{Value: false},
	}
)

var (
	openFlagsStrings          = map[int]string{}
	chmodModeStrings          = map[int]string{}
	unlinkFlagsStrings        = map[int]string{}
	kernelCapabilitiesStrings = map[int]string{}
)

func initOpenConstants() {
	for k, v := range openFlagsConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
	}

	for k, v := range openFlagsConstants {
		openFlagsStrings[v] = k
	}
}

func initChmodConstants() {
	for k, v := range chmodModeConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
		chmodModeStrings[v] = k
	}
}

func initUnlinkConstanst() {
	for k, v := range unlinkFlagsConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
		unlinkFlagsStrings[v] = k
	}
}

func initErrorConstants() {
	for k, v := range errorConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
	}
}

func initKernelCapabilityConstants() {
	for k, v := range kernelCapabilityConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
		kernelCapabilitiesStrings[v] = k
	}
}

func initConstants() {
	initErrorConstants()
	initOpenConstants()
	initChmodConstants()
	initUnlinkConstanst()
	initKernelCapabilityConstants()
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

// OpenFlags represents an open flags bitmask value
type OpenFlags int

func (f OpenFlags) String() string {
	if int(f) == syscall.O_RDONLY {
		return openFlagsStrings[syscall.O_RDONLY]
	}
	return bitmaskToString(int(f), openFlagsStrings)
}

// StringArray returns the open flags as an array of strings
func (f OpenFlags) StringArray() []string {
	if int(f) == syscall.O_RDONLY {
		return []string{openFlagsStrings[syscall.O_RDONLY]}
	}
	return bitmaskToStringArray(int(f), openFlagsStrings)
}

// ChmodMode represent a chmod mode bitmask value
type ChmodMode int

func (m ChmodMode) String() string {
	return bitmaskToString(int(m), chmodModeStrings)
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

// RetValError represents a syscall return error value
type RetValError int

func (f RetValError) String() string {
	v := int(f)
	if v < 0 {
		return syscall.Errno(-v).Error()
	}
	return ""
}

func init() {
	initConstants()
}

// KernelCapability represents a kernel capability bitmask value
type KernelCapability uint64

func (kc KernelCapability) String() string {
	return bitmaskToString(int(kc), kernelCapabilitiesStrings)
}

// StringArray returns the kernel capabilities as an array of strings
func (kc KernelCapability) StringArray() []string {
	return bitmaskToStringArray(int(kc), kernelCapabilitiesStrings)
}
