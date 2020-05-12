package probe

import (
	"encoding/binary"
	"unsafe"
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
