// Package notify defines directory change notification filters and results.
package notify

import "github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"

// Filter selects the directory changes reported by WaitForChange.
type Filter uint32

const (
	FileName    Filter = wire.FILE_NOTIFY_CHANGE_FILE_NAME
	DirName     Filter = wire.FILE_NOTIFY_CHANGE_DIR_NAME
	Attributes  Filter = wire.FILE_NOTIFY_CHANGE_ATTRIBUTES
	Size        Filter = wire.FILE_NOTIFY_CHANGE_SIZE
	LastWrite   Filter = wire.FILE_NOTIFY_CHANGE_LAST_WRITE
	LastAccess  Filter = wire.FILE_NOTIFY_CHANGE_LAST_ACCESS
	Creation    Filter = wire.FILE_NOTIFY_CHANGE_CREATION
	EA          Filter = wire.FILE_NOTIFY_CHANGE_EA
	Security    Filter = wire.FILE_NOTIFY_CHANGE_SECURITY
	StreamName  Filter = wire.FILE_NOTIFY_CHANGE_STREAM_NAME
	StreamSize  Filter = wire.FILE_NOTIFY_CHANGE_STREAM_SIZE
	StreamWrite Filter = wire.FILE_NOTIFY_CHANGE_STREAM_WRITE
)

// Action identifies the change described by an Event.
type Action uint32

const (
	Added                Action = wire.FILE_ACTION_ADDED
	Removed              Action = wire.FILE_ACTION_REMOVED
	Modified             Action = wire.FILE_ACTION_MODIFIED
	RenamedOldName       Action = wire.FILE_ACTION_RENAMED_OLD_NAME
	RenamedNewName       Action = wire.FILE_ACTION_RENAMED_NEW_NAME
	AddedStream          Action = wire.FILE_ACTION_ADDED_STREAM
	RemovedStream        Action = wire.FILE_ACTION_REMOVED_STREAM
	ModifiedStream       Action = wire.FILE_ACTION_MODIFIED_STREAM
	RemovedByDelete      Action = wire.FILE_ACTION_REMOVED_BY_DELETE
	IDNotTunnelled       Action = wire.FILE_ACTION_ID_NOT_TUNNELLED
	TunnelledIDCollision Action = wire.FILE_ACTION_TUNNELLED_ID_COLLISION
)

// Event is one directory change. Name is relative to the monitored
// directory and is not normalized or joined with the File name.
type Event struct {
	Action Action
	Name   string
}

// Result is the result of one WaitForChange request. A canceled request may
// have consumed a server notification, and notification results do not
// guarantee a complete change history; callers issue the next request when
// they want to continue monitoring.
type Result struct {
	Events         []Event
	RescanRequired bool
}
