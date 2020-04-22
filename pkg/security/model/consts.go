package model

// SetAttrFlag - Set Attr flag
type SetAttrFlag uint32

const (
	// AttrMode - Mode changed
	AttrMode SetAttrFlag = 1 << 0
	// AttrUID - UID changed
	AttrUID SetAttrFlag = 1 << 1
	// AttrGID - GID changed
	AttrGID SetAttrFlag = 1 << 2
	// AttrSize - Size changed
	AttrSize SetAttrFlag = 1 << 3
	// AttrAtime - Atime changed
	AttrAtime SetAttrFlag = 1 << 4
	// AttrMtime - Mtime changed
	AttrMtime SetAttrFlag = 1 << 5
	// AttrCtime - Ctime changed
	AttrCtime SetAttrFlag = 1 << 6
	// AttrAtimeSet - ATimeSet
	AttrAtimeSet SetAttrFlag = 1 << 7
	// AttrMTimeSet - MTimeSet
	AttrMTimeSet SetAttrFlag = 1 << 8
	// AttrForce - Not a change, but a change it
	AttrForce SetAttrFlag = 1 << 9
	// AttrKillSUID - Kill SUID
	AttrKillSUID SetAttrFlag = 1 << 11
	// AttrKillSGID - Kill SGID
	AttrKillSGID SetAttrFlag = 1 << 12
	// AttrFile - File changed
	AttrFile SetAttrFlag = 1 << 13
	// AttrKillPriv - Fill Priv
	AttrKillPriv SetAttrFlag = 1 << 14
	// AttrOpen - Open
	AttrOpen SetAttrFlag = 1 << 15
	// AttrTimesSet - TimesSet
	AttrTimesSet SetAttrFlag = 1 << 16
	// AttrTouch - Touch
	AttrTouch SetAttrFlag = 1 << 17
)
