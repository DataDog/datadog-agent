package probe

import (
	"encoding/binary"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type ProbeEventType uint64

const (
	// FileOpenEventType - File open event
	FileOpenEventType ProbeEventType = iota + 1
	// FileMkdirEventType - Folder creation event
	FileMkdirEventType
	// FileHardLinkEventType - Hard link creation event
	FileHardLinkEventType
	// FileRenameEventType - File or folder rename event
	FileRenameEventType
	// FileSetAttrEventType - Set Attr event
	FileSetAttrEventType
	// FileUnlinkEventType - Unlink event
	FileUnlinkEventType
	// FileRmdirEventType - Rmdir event
	FileRmdirEventType
)

func (t ProbeEventType) String() string {
	switch t {
	case FileOpenEventType:
		return "open"
	case FileMkdirEventType:
		return "mkdir"
	case FileHardLinkEventType:
	case FileRenameEventType:
		return "rename"
	case FileSetAttrEventType:
	case FileUnlinkEventType:
		return "unlink"
	case FileRmdirEventType:
		return "rmdir"
	}
	return "unknown"
}

var (
	SECLConstants = map[string]interface{}{
		// boolean
		"true":  &eval.BoolEvaluator{Value: true},
		"false": &eval.BoolEvaluator{Value: false},

		// errors
		"E2BIG":           &eval.IntEvaluator{Value: -int(syscall.E2BIG)},
		"EACCES":          &eval.IntEvaluator{Value: -int(syscall.EACCES)},
		"EADDRINUSE":      &eval.IntEvaluator{Value: -int(syscall.EADDRINUSE)},
		"EADDRNOTAVAIL":   &eval.IntEvaluator{Value: -int(syscall.EADDRNOTAVAIL)},
		"EADV":            &eval.IntEvaluator{Value: -int(syscall.EADV)},
		"EAFNOSUPPORT":    &eval.IntEvaluator{Value: -int(syscall.EAFNOSUPPORT)},
		"EAGAIN":          &eval.IntEvaluator{Value: -int(syscall.EAGAIN)},
		"EALREADY":        &eval.IntEvaluator{Value: -int(syscall.EALREADY)},
		"EBADE":           &eval.IntEvaluator{Value: -int(syscall.EBADE)},
		"EBADF":           &eval.IntEvaluator{Value: -int(syscall.EBADF)},
		"EBADFD":          &eval.IntEvaluator{Value: -int(syscall.EBADFD)},
		"EBADMSG":         &eval.IntEvaluator{Value: -int(syscall.EBADMSG)},
		"EBADR":           &eval.IntEvaluator{Value: -int(syscall.EBADR)},
		"EBADRQC":         &eval.IntEvaluator{Value: -int(syscall.EBADRQC)},
		"EBADSLT":         &eval.IntEvaluator{Value: -int(syscall.EBADSLT)},
		"EBFONT":          &eval.IntEvaluator{Value: -int(syscall.EBFONT)},
		"EBUSY":           &eval.IntEvaluator{Value: -int(syscall.EBUSY)},
		"ECANCELED":       &eval.IntEvaluator{Value: -int(syscall.ECANCELED)},
		"ECHILD":          &eval.IntEvaluator{Value: -int(syscall.ECHILD)},
		"ECHRNG":          &eval.IntEvaluator{Value: -int(syscall.ECHRNG)},
		"ECOMM":           &eval.IntEvaluator{Value: -int(syscall.ECOMM)},
		"ECONNABORTED":    &eval.IntEvaluator{Value: -int(syscall.ECONNABORTED)},
		"ECONNREFUSED":    &eval.IntEvaluator{Value: -int(syscall.ECONNREFUSED)},
		"ECONNRESET":      &eval.IntEvaluator{Value: -int(syscall.ECONNRESET)},
		"EDEADLK":         &eval.IntEvaluator{Value: -int(syscall.EDEADLK)},
		"EDEADLOCK":       &eval.IntEvaluator{Value: -int(syscall.EDEADLOCK)},
		"EDESTADDRREQ":    &eval.IntEvaluator{Value: -int(syscall.EDESTADDRREQ)},
		"EDOM":            &eval.IntEvaluator{Value: -int(syscall.EDOM)},
		"EDOTDOT":         &eval.IntEvaluator{Value: -int(syscall.EDOTDOT)},
		"EDQUOT":          &eval.IntEvaluator{Value: -int(syscall.EDQUOT)},
		"EEXIST":          &eval.IntEvaluator{Value: -int(syscall.EEXIST)},
		"EFAULT":          &eval.IntEvaluator{Value: -int(syscall.EFAULT)},
		"EFBIG":           &eval.IntEvaluator{Value: -int(syscall.EFBIG)},
		"EHOSTDOWN":       &eval.IntEvaluator{Value: -int(syscall.EHOSTDOWN)},
		"EHOSTUNREACH":    &eval.IntEvaluator{Value: -int(syscall.EHOSTUNREACH)},
		"EIDRM":           &eval.IntEvaluator{Value: -int(syscall.EIDRM)},
		"EILSEQ":          &eval.IntEvaluator{Value: -int(syscall.EIDRM)},
		"EINPROGRESS":     &eval.IntEvaluator{Value: -int(syscall.EINPROGRESS)},
		"EINTR":           &eval.IntEvaluator{Value: -int(syscall.EINTR)},
		"EINVAL":          &eval.IntEvaluator{Value: -int(syscall.EINVAL)},
		"EIO":             &eval.IntEvaluator{Value: -int(syscall.EIO)},
		"EISCONN":         &eval.IntEvaluator{Value: -int(syscall.EISCONN)},
		"EISDIR":          &eval.IntEvaluator{Value: -int(syscall.EISDIR)},
		"EISNAM":          &eval.IntEvaluator{Value: -int(syscall.EISNAM)},
		"EKEYEXPIRED":     &eval.IntEvaluator{Value: -int(syscall.EKEYEXPIRED)},
		"EKEYREJECTED":    &eval.IntEvaluator{Value: -int(syscall.EKEYREJECTED)},
		"EKEYREVOKED":     &eval.IntEvaluator{Value: -int(syscall.EKEYREVOKED)},
		"EL2HLT":          &eval.IntEvaluator{Value: -int(syscall.EL2HLT)},
		"EL2NSYNC":        &eval.IntEvaluator{Value: -int(syscall.EL2NSYNC)},
		"EL3HLT":          &eval.IntEvaluator{Value: -int(syscall.EL3HLT)},
		"EL3RST":          &eval.IntEvaluator{Value: -int(syscall.EL3RST)},
		"ELIBACC":         &eval.IntEvaluator{Value: -int(syscall.ELIBACC)},
		"ELIBBAD":         &eval.IntEvaluator{Value: -int(syscall.ELIBBAD)},
		"ELIBEXEC":        &eval.IntEvaluator{Value: -int(syscall.ELIBEXEC)},
		"ELIBMAX":         &eval.IntEvaluator{Value: -int(syscall.ELIBMAX)},
		"ELIBSCN":         &eval.IntEvaluator{Value: -int(syscall.ELIBSCN)},
		"ELNRNG":          &eval.IntEvaluator{Value: -int(syscall.ELNRNG)},
		"ELOOP":           &eval.IntEvaluator{Value: -int(syscall.ELOOP)},
		"EMEDIUMTYPE":     &eval.IntEvaluator{Value: -int(syscall.EMEDIUMTYPE)},
		"EMFILE":          &eval.IntEvaluator{Value: -int(syscall.EMFILE)},
		"EMLINK":          &eval.IntEvaluator{Value: -int(syscall.EMLINK)},
		"EMSGSIZE":        &eval.IntEvaluator{Value: -int(syscall.EMSGSIZE)},
		"EMULTIHOP":       &eval.IntEvaluator{Value: -int(syscall.EMULTIHOP)},
		"ENAMETOOLONG":    &eval.IntEvaluator{Value: -int(syscall.ENAMETOOLONG)},
		"ENAVAIL":         &eval.IntEvaluator{Value: -int(syscall.ENAVAIL)},
		"ENETDOWN":        &eval.IntEvaluator{Value: -int(syscall.ENETDOWN)},
		"ENETRESET":       &eval.IntEvaluator{Value: -int(syscall.ENETRESET)},
		"ENETUNREACH":     &eval.IntEvaluator{Value: -int(syscall.ENETUNREACH)},
		"ENFILE":          &eval.IntEvaluator{Value: -int(syscall.ENFILE)},
		"ENOANO":          &eval.IntEvaluator{Value: -int(syscall.ENOANO)},
		"ENOBUFS":         &eval.IntEvaluator{Value: -int(syscall.ENOBUFS)},
		"ENOCSI":          &eval.IntEvaluator{Value: -int(syscall.ENOCSI)},
		"ENODATA":         &eval.IntEvaluator{Value: -int(syscall.ENODATA)},
		"ENODEV":          &eval.IntEvaluator{Value: -int(syscall.ENODEV)},
		"ENOENT":          &eval.IntEvaluator{Value: -int(syscall.ENOENT)},
		"ENOEXEC":         &eval.IntEvaluator{Value: -int(syscall.ENOEXEC)},
		"ENOKEY":          &eval.IntEvaluator{Value: -int(syscall.ENOKEY)},
		"ENOLCK":          &eval.IntEvaluator{Value: -int(syscall.ENOLCK)},
		"ENOLINK":         &eval.IntEvaluator{Value: -int(syscall.ENOLINK)},
		"ENOMEDIUM":       &eval.IntEvaluator{Value: -int(syscall.ENOMEDIUM)},
		"ENOMEM":          &eval.IntEvaluator{Value: -int(syscall.ENOMEM)},
		"ENOMSG":          &eval.IntEvaluator{Value: -int(syscall.ENOMSG)},
		"ENONET":          &eval.IntEvaluator{Value: -int(syscall.ENONET)},
		"ENOPKG":          &eval.IntEvaluator{Value: -int(syscall.ENOPKG)},
		"ENOPROTOOPT":     &eval.IntEvaluator{Value: -int(syscall.ENOPROTOOPT)},
		"ENOSPC":          &eval.IntEvaluator{Value: -int(syscall.ENOSPC)},
		"ENOSR":           &eval.IntEvaluator{Value: -int(syscall.ENOSR)},
		"ENOSTR":          &eval.IntEvaluator{Value: -int(syscall.ENOSTR)},
		"ENOSYS":          &eval.IntEvaluator{Value: -int(syscall.ENOSYS)},
		"ENOTBLK":         &eval.IntEvaluator{Value: -int(syscall.ENOTBLK)},
		"ENOTCONN":        &eval.IntEvaluator{Value: -int(syscall.ENOTCONN)},
		"ENOTDIR":         &eval.IntEvaluator{Value: -int(syscall.ENOTDIR)},
		"ENOTEMPTY":       &eval.IntEvaluator{Value: -int(syscall.ENOTEMPTY)},
		"ENOTNAM":         &eval.IntEvaluator{Value: -int(syscall.ENOTNAM)},
		"ENOTRECOVERABLE": &eval.IntEvaluator{Value: -int(syscall.ENOTRECOVERABLE)},
		"ENOTSOCK":        &eval.IntEvaluator{Value: -int(syscall.ENOTSOCK)},
		"ENOTSUP":         &eval.IntEvaluator{Value: -int(syscall.ENOTSUP)},
		"ENOTTY":          &eval.IntEvaluator{Value: -int(syscall.ENOTTY)},
		"ENOTUNIQ":        &eval.IntEvaluator{Value: -int(syscall.ENOTUNIQ)},
		"ENXIO":           &eval.IntEvaluator{Value: -int(syscall.ENXIO)},
		"EOPNOTSUPP":      &eval.IntEvaluator{Value: -int(syscall.EOPNOTSUPP)},
		"EOVERFLOW":       &eval.IntEvaluator{Value: -int(syscall.EOVERFLOW)},
		"EOWNERDEAD":      &eval.IntEvaluator{Value: -int(syscall.EOWNERDEAD)},
		"EPERM":           &eval.IntEvaluator{Value: -int(syscall.EPERM)},
		"EPFNOSUPPORT":    &eval.IntEvaluator{Value: -int(syscall.EPFNOSUPPORT)},
		"EPIPE":           &eval.IntEvaluator{Value: -int(syscall.EPIPE)},
		"EPROTO":          &eval.IntEvaluator{Value: -int(syscall.EPROTO)},
		"EPROTONOSUPPORT": &eval.IntEvaluator{Value: -int(syscall.EPROTONOSUPPORT)},
		"EPROTOTYPE":      &eval.IntEvaluator{Value: -int(syscall.EPROTOTYPE)},
		"ERANGE":          &eval.IntEvaluator{Value: -int(syscall.ERANGE)},
		"EREMCHG":         &eval.IntEvaluator{Value: -int(syscall.EREMCHG)},
		"EREMOTE":         &eval.IntEvaluator{Value: -int(syscall.EREMOTE)},
		"EREMOTEIO":       &eval.IntEvaluator{Value: -int(syscall.EREMOTEIO)},
		"ERESTART":        &eval.IntEvaluator{Value: -int(syscall.ERESTART)},
		"ERFKILL":         &eval.IntEvaluator{Value: -int(syscall.ERFKILL)},
		"EROFS":           &eval.IntEvaluator{Value: -int(syscall.EROFS)},
		"ESHUTDOWN":       &eval.IntEvaluator{Value: -int(syscall.ESHUTDOWN)},
		"ESOCKTNOSUPPORT": &eval.IntEvaluator{Value: -int(syscall.ESOCKTNOSUPPORT)},
		"ESPIPE":          &eval.IntEvaluator{Value: -int(syscall.ESPIPE)},
		"ESRCH":           &eval.IntEvaluator{Value: -int(syscall.ESRCH)},
		"ESRMNT":          &eval.IntEvaluator{Value: -int(syscall.ESRMNT)},
		"ESTALE":          &eval.IntEvaluator{Value: -int(syscall.ESTALE)},
		"ESTRPIPE":        &eval.IntEvaluator{Value: -int(syscall.ESTRPIPE)},
		"ETIME":           &eval.IntEvaluator{Value: -int(syscall.ETIME)},
		"ETIMEDOUT":       &eval.IntEvaluator{Value: -int(syscall.ETIMEDOUT)},
		"ETOOMANYREFS":    &eval.IntEvaluator{Value: -int(syscall.ETOOMANYREFS)},
		"ETXTBSY":         &eval.IntEvaluator{Value: -int(syscall.ETXTBSY)},
		"EUCLEAN":         &eval.IntEvaluator{Value: -int(syscall.EUCLEAN)},
		"EUNATCH":         &eval.IntEvaluator{Value: -int(syscall.EUNATCH)},
		"EUSERS":          &eval.IntEvaluator{Value: -int(syscall.EUSERS)},
		"EWOULDBLOCK":     &eval.IntEvaluator{Value: -int(syscall.EWOULDBLOCK)},
		"EXDEV":           &eval.IntEvaluator{Value: -int(syscall.EXDEV)},
		"EXFULL":          &eval.IntEvaluator{Value: -int(syscall.EXFULL)},

		// open flags
		"O_RDONLY": &eval.IntEvaluator{Value: syscall.O_RDONLY},
		"O_WRONLY": &eval.IntEvaluator{Value: syscall.O_WRONLY},
		"O_RDWR":   &eval.IntEvaluator{Value: syscall.O_RDWR},
		"O_APPEND": &eval.IntEvaluator{Value: syscall.O_APPEND},
		"O_CREAT":  &eval.IntEvaluator{Value: syscall.O_CREAT},
		"O_EXCL":   &eval.IntEvaluator{Value: syscall.O_EXCL},
		"O_SYNC":   &eval.IntEvaluator{Value: syscall.O_SYNC},
		"O_TRUNC":  &eval.IntEvaluator{Value: syscall.O_TRUNC},

		// permissions
		"S_IEXEC":  &eval.IntEvaluator{Value: syscall.S_IEXEC},
		"S_IFBLK":  &eval.IntEvaluator{Value: syscall.S_IFBLK},
		"S_IFCHR":  &eval.IntEvaluator{Value: syscall.S_IFCHR},
		"S_IFDIR":  &eval.IntEvaluator{Value: syscall.S_IFDIR},
		"S_IFIFO":  &eval.IntEvaluator{Value: syscall.S_IFIFO},
		"S_IFLNK":  &eval.IntEvaluator{Value: syscall.S_IFLNK},
		"S_IFMT":   &eval.IntEvaluator{Value: syscall.S_IFMT},
		"S_IFREG":  &eval.IntEvaluator{Value: syscall.S_IFREG},
		"S_IFSOCK": &eval.IntEvaluator{Value: syscall.S_IFSOCK},
		"S_IREAD":  &eval.IntEvaluator{Value: syscall.S_IREAD},
		"S_IRGRP":  &eval.IntEvaluator{Value: syscall.S_IRGRP},
		"S_IROTH":  &eval.IntEvaluator{Value: syscall.S_IROTH},
		"S_IRUSR":  &eval.IntEvaluator{Value: syscall.S_IRUSR},
		"S_IRWXG":  &eval.IntEvaluator{Value: syscall.S_IRWXG},
		"S_IRWXO":  &eval.IntEvaluator{Value: syscall.S_IRWXO},
		"S_IRWXU":  &eval.IntEvaluator{Value: syscall.S_IRWXU},
		"S_ISGID":  &eval.IntEvaluator{Value: syscall.S_ISGID},
		"S_ISUID":  &eval.IntEvaluator{Value: syscall.S_ISUID},
		"S_ISVTX":  &eval.IntEvaluator{Value: syscall.S_ISVTX},
		"S_IWGRP":  &eval.IntEvaluator{Value: syscall.S_IWGRP},
		"S_IWOTH":  &eval.IntEvaluator{Value: syscall.S_IWOTH},
		"S_IWRITE": &eval.IntEvaluator{Value: syscall.S_IWRITE},
		"S_IWUSR":  &eval.IntEvaluator{Value: syscall.S_IWUSR},
		"S_IXGRP":  &eval.IntEvaluator{Value: syscall.S_IXGRP},
		"S_IXOTH":  &eval.IntEvaluator{Value: syscall.S_IXOTH},
		"S_IXUSR":  &eval.IntEvaluator{Value: syscall.S_IXUSR},
	}
)

func getHostByteOrder() binary.ByteOrder {
	var i int32 = 0x01020304
	u := unsafe.Pointer(&i)
	pb := (*byte)(u)
	b := *pb
	if b == 0x04 {
		return binary.LittleEndian
	}

	return binary.BigEndian
}

var byteOrder binary.ByteOrder

func init() {
	byteOrder = getHostByteOrder()
}
