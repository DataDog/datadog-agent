package probe

import (
	"C"

	"bytes"
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	ebpf "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/iovisor/gobpf/bcc"
)

type EventHandler interface {
	HandleEvent(event interface{})
}

type Probe struct {
	*ebpf.Probe
	handler EventHandler
}

func NewProbe(handler EventHandler) *Probe {
	p := &Probe{handler: handler}

	p.Probe = &ebpf.Probe{
		Source: source,
		Tables: map[string]*ebpf.Table{
			"pathnames":       &ebpf.Table{},
			"process_filter":  &ebpf.Table{},
			"filter_settings": &ebpf.Table{},
			"pid_filter":      &ebpf.Table{},
			"pidns_filter":    &ebpf.Table{},
		},
		Hooks: []ebpf.Hook{
			&ebpf.KProbe{
				Name:       "may_open",
				EntryFunc:  "trace_may_open",
				EntryEvent: "may_open.isra.0",
				ExitFunc:   "trace_ret_may_open",
				ExitEvent:  "may_open.isra.0",
			},
			&ebpf.KProbe{
				Name:       "vfs_mkdir",
				EntryFunc:  "trace_vfs_mkdir",
				EntryEvent: "vfs_mkdir",
				ExitFunc:   "trace_ret_vfs_mkdir",
				ExitEvent:  "vfs_mkdir",
			},
			&ebpf.KProbe{
				Name:       "vfs_link",
				EntryFunc:  "trace_vfs_link",
				EntryEvent: "vfs_link",
				ExitFunc:   "trace_ret_vfs_link",
				ExitEvent:  "vfs_link",
			},
			&ebpf.KProbe{
				Name:       "vfs_rename",
				EntryFunc:  "trace_vfs_rename",
				EntryEvent: "vfs_rename",
				ExitFunc:   "trace_ret_vfs_rename",
				ExitEvent:  "vfs_rename",
			},
			&ebpf.KProbe{
				Name:       "unlink_tracker",
				EntryFunc:  "trace_vfs_unlink",
				EntryEvent: "vfs_unlink",
				ExitFunc:   "trace_ret_vfs_unlink",
				ExitEvent:  "vfs_unlink",
			},
			&ebpf.KProbe{
				Name:       "rmdir_tracker",
				EntryFunc:  "trace_vfs_rmdir",
				EntryEvent: "vfs_rmdir",
				ExitFunc:   "trace_ret_vfs_rmdir",
				ExitEvent:  "vfs_rmdir",
			},
			&ebpf.KProbe{
				Name:       "setattr_tracker",
				EntryFunc:  "trace_security_inode_setattr",
				EntryEvent: "security_inode_setattr",
				ExitFunc:   "trace_ret_security_inode_setattr",
				ExitEvent:  "security_inode_setattr",
			},
		},
		PerfMaps: []*ebpf.PerfMap{
			&ebpf.PerfMap{
				Name:    "dentry_events",
				Handler: p.handleDentryEvent,
			},
			&ebpf.PerfMap{
				Name:    "setattr_events",
				Handler: p.handleSecurityInodeSetattr,
			},
		},
	}

	return p
}

func (p *Probe) DispatchEvent(event interface{}) {
	p.handler.HandleEvent(event)
}

// handleDentryEvent - Handles a dentry event
func (p *Probe) handleDentryEvent(data []byte) {
	// Decode data from probe
	var eventRaw DentryEventRaw
	err := binary.Read(bytes.NewBuffer(data), bcc.GetHostByteOrder(), &eventRaw)
	if err != nil {
		fmt.Printf("failed to decode received data (dentryEvent): %s\n", err)
		return
	}

	// Prepare event
	event := DentryEvent{
		EventBase: EventBase{
			EventType:        eventRaw.GetProbeEventType(),
			EventMonitorName: FimMonitor,
			EventMonitorType: EBPF,
			Timestamp:        p.StartTime.Add(time.Duration(eventRaw.TimestampRaw) * time.Nanosecond),
		},
		DentryEventRaw: &eventRaw,
		TTYName:        C.GoString((*C.char)(unsafe.Pointer(&eventRaw.TTYNameRaw))),
	}
	event.SrcFilename, err = p.resolveDentryPath(eventRaw.SrcPathnameKey)
	if err != nil {
		fmt.Printf("failed to resolve dentry path: %v", err)
	}

	switch event.EventType {
	case FileHardLinkEventType, FileRenameEventType:
		event.TargetFilename, err = p.resolveDentryPath(eventRaw.TargetPathnameKey)
		if err != nil {
			fmt.Printf("failed to resolve dentry path: %v", err)
		}
	}

	// Dispatch dentry event
	p.DispatchEvent(&event)
}

// handleSecurityInodeSetattr - Handles a setattr update event
func (p *Probe) handleSecurityInodeSetattr(data []byte) {
	// Parse data from probe
	var eventRaw SetAttrRaw
	err := binary.Read(bytes.NewBuffer(data), bcc.GetHostByteOrder(), &eventRaw)
	if err != nil {
		fmt.Printf("failed to decode received data: %s\n", err)
		return
	}
	// Prepare event
	event := SetAttrEvent{
		EventBase: EventBase{
			EventType:        FileSetAttrEventType,
			EventMonitorName: FimMonitor,
			EventMonitorType: EBPF,
			Timestamp:        p.StartTime.Add(time.Duration(eventRaw.TimestampRaw) * time.Nanosecond),
		},
		SetAttrRaw: &eventRaw,
		TTYName:    C.GoString((*C.char)(unsafe.Pointer(&eventRaw.TTYNameRaw))),
		Atime:      time.Unix(eventRaw.AtimeRaw[0], eventRaw.AtimeRaw[1]),
		Mtime:      time.Unix(eventRaw.MtimeRaw[0], eventRaw.MtimeRaw[1]),
		Ctime:      time.Unix(eventRaw.CtimeRaw[0], eventRaw.CtimeRaw[1]),
	}
	event.Pathname, err = p.resolveDentryPath(eventRaw.PathnameKey)
	if err != nil {
		fmt.Printf("failed to resolve dentry path (setAttr): %v", err)
	}

	// Filter and dispatch the event
	p.DispatchEvent(&event)
}

// resolveDentryPath - Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (p *Probe) resolveDentryPath(pathnameKey uint32) (string, error) {
	// Don't resolve path if pathnameKey isn't valid
	if pathnameKey <= 0 {
		return "", fmt.Errorf("invalid pathname key %v", pathnameKey)
	}
	table := p.Tables["pathnames"]
	if table == nil {
		return "", fmt.Errorf("pathnames BPF_HASH table doesn't exist")
	}
	// Convert key into bytes
	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, pathnameKey)
	filename := ""
	done := false
	pathRaw := []byte{}
	var path struct {
		ParentKey uint32
		Name      [255]byte
	}
	var err1, err2 error
	// Fetch path recursively
	for !done {
		pathRaw, err1 = table.Get(key)
		if err1 != nil {
			filename = "*ERROR*" + filename
			done = true
			continue
		}
		err1 = binary.Read(bytes.NewBuffer(pathRaw), bcc.GetHostByteOrder(), &path)
		if err1 != nil {
			err1 = fmt.Errorf("failed to decode received data (pathLeaf): %s", err1)
			done = true
		}
		// Delete key
		if err2 = table.Delete(key); err2 != nil {
			err1 = fmt.Errorf("pathnames map deletion error: %v", err2)
		}
		if done {
			continue
		}
		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if path.Name[0] != 47 {
			filename = "/" + C.GoString((*C.char)(unsafe.Pointer(&path.Name))) + filename
		}
		if path.ParentKey == 0 {
			done = true
			continue
		}
		// Prepare next key
		binary.LittleEndian.PutUint32(key, path.ParentKey)
	}
	if len(filename) == 0 {
		filename = "/"
	}
	return filename, err1
}
