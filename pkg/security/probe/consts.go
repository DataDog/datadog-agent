package probe

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
	case FileOpenEventType,
		FileMkdirEventType,
		FileHardLinkEventType,
		FileRenameEventType,
		FileSetAttrEventType,
		FileUnlinkEventType,
		FileRmdirEventType:
		return "fs"
	default:
		return "unknown"
	}
}
