// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
	"sort"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"golang.org/x/sys/unix"
)

// EventType describes the type of an event sent from the kernel
type EventType uint64

const (
	// UnknownEventType - unknow event
	UnknownEventType EventType = iota
	// FileOpenEventType - File open event
	FileOpenEventType
	// FileMkdirEventType - Folder creation event
	FileMkdirEventType
	// FileLinkEventType - Hard link creation event
	FileLinkEventType
	// FileRenameEventType - File or folder rename event
	FileRenameEventType
	// FileUnlinkEventType - Unlink event
	FileUnlinkEventType
	// FileRmdirEventType - Rmdir event
	FileRmdirEventType
	// FileChmodEventType - Chmod event
	FileChmodEventType
	// FileChownEventType - Chown event
	FileChownEventType
	// FileUtimeEventType - Utime event
	FileUtimeEventType
	// FileMountEventType - Mount event
	FileMountEventType
	// FileUmountEventType - Umount event
	FileUmountEventType
	// FileSetXAttrEventType - Setxattr event
	FileSetXAttrEventType
	// FileRemoveXAttrEventType - Removexattr event
	FileRemoveXAttrEventType
	// internalEventType - used internally to get the maximum number of event. Has to be the last one
	maxEventType
)

func (t EventType) String() string {
	switch t {
	case FileOpenEventType:
		return "open"
	case FileMkdirEventType:
		return "mkdir"
	case FileLinkEventType:
		return "link"
	case FileRenameEventType:
		return "rename"
	case FileUnlinkEventType:
		return "unlink"
	case FileRmdirEventType:
		return "rmdir"
	case FileChmodEventType:
		return "chmod"
	case FileChownEventType:
		return "chown"
	case FileUtimeEventType:
		return "utimes"
	case FileMountEventType:
		return "mount"
	case FileUmountEventType:
		return "umount"
	case FileSetXAttrEventType:
		return "setxattr"
	case FileRemoveXAttrEventType:
		return "removexattr"
	}
	return "unknown"
}

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
		"O_LARGEFILE": syscall.O_LARGEFILE,
		"O_NDELAY":    syscall.O_NDELAY,
		"O_NOATIME":   syscall.O_NOATIME,
		"O_NOCTTY":    syscall.O_NOCTTY,
		"O_NOFOLLOW":  syscall.O_NOFOLLOW,
		"O_NONBLOCK":  syscall.O_NONBLOCK,
		"O_RSYNC":     syscall.O_RSYNC,
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
	openFlagsStrings   = map[int]string{}
	chmodModeStrings   = map[int]string{}
	unlinkFlagsStrings = map[int]string{}
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
	}

	for k, v := range chmodModeConstants {
		chmodModeStrings[v] = k
	}
}

func initUnlinkConstanst() {
	for k, v := range unlinkFlagsConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
	}

	for k, v := range unlinkFlagsConstants {
		unlinkFlagsStrings[v] = k
	}
}

func initErrorConstants() {
	for k, v := range errorConstants {
		SECLConstants[k] = &eval.IntEvaluator{Value: v}
	}
}

func initConstants() {
	initErrorConstants()
	initOpenConstants()
	initChmodConstants()
	initUnlinkConstanst()
}

func bitmaskToString(bitmask int, intToStrMap map[int]string) string {
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
		strs = append(strs, fmt.Sprintf("%d", bitmask^result))
	}

	sort.Strings(strs)

	return strings.Join(strs, " | ")
}

// OpenFlags represents an open flags bitmask value
type OpenFlags int

func (f OpenFlags) String() string {
	return bitmaskToString(int(f), openFlagsStrings)
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
