package fim

import (
	"C"
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	"github.com/iovisor/gobpf/bcc"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
)

// Monitor - eBPF FIM event monitor
var Monitor = &probe.GenericEventMonitor{
	Name:   probe.FimMonitor,
	Type:   probe.EBPF,
	Source: source,
	TableNames: []string{
		"pathnames",
		"pid_filter",
		"pidns_filter",
		"filter_settings",
	},
	Probes: []*probe.Probe{
		&probe.Probe{
			Name:            "may_open",
			Type:            probe.KProbeType,
			EntryFunc:       "trace_may_open",
			EntryEvent:      "may_open.isra.0",
			ExitFunc:        "trace_ret_may_open",
			ExitEvent:       "may_open.isra.0",
			KProbeMaxActive: -1,
			PerfMaps: []*probe.PerfMap{
				&probe.PerfMap{
					UserSpaceBufferLen:  1000,
					PerfOutputTableName: "dentry_events",
					DataHandler:         HandleDentryEvent,
				},
			},
		},
		&probe.Probe{
			Name:            "vfs_mkdir",
			Type:            probe.KProbeType,
			EntryFunc:       "trace_vfs_mkdir",
			EntryEvent:      "vfs_mkdir",
			ExitFunc:        "trace_ret_vfs_mkdir",
			ExitEvent:       "vfs_mkdir",
			KProbeMaxActive: -1,
		},
		&probe.Probe{
			Name:            "vfs_link",
			Type:            probe.KProbeType,
			EntryFunc:       "trace_vfs_link",
			EntryEvent:      "vfs_link",
			ExitFunc:        "trace_ret_vfs_link",
			ExitEvent:       "vfs_link",
			KProbeMaxActive: -1,
		},
		&probe.Probe{
			Name:            "vfs_rename",
			Type:            probe.KProbeType,
			EntryFunc:       "trace_vfs_rename",
			EntryEvent:      "vfs_rename",
			ExitFunc:        "trace_ret_vfs_rename",
			ExitEvent:       "vfs_rename",
			KProbeMaxActive: -1,
		},
		&probe.Probe{
			Name:            "unlink_tracker",
			Type:            probe.KProbeType,
			EntryFunc:       "trace_vfs_unlink",
			EntryEvent:      "vfs_unlink",
			ExitFunc:        "trace_ret_vfs_unlink",
			ExitEvent:       "vfs_unlink",
			KProbeMaxActive: -1,
		},
		&probe.Probe{
			Name:            "rmdir_tracker",
			Type:            probe.KProbeType,
			EntryFunc:       "trace_vfs_rmdir",
			EntryEvent:      "vfs_rmdir",
			ExitFunc:        "trace_ret_vfs_rmdir",
			ExitEvent:       "vfs_rmdir",
			KProbeMaxActive: -1,
		},
		// &emonitor.Probe{
		// 	Name:            "file_modify",
		// 	Type:            emonitor.KProbeType,
		// 	EntryFunc:       "trace_fsnotify_parent",
		// 	EntryEvent:      "__fsnotify_parent",
		// 	ExitFunc:        "trace_ret_fsnotify_parent",
		// 	ExitEvent:       "__fsnotify_parent",
		// 	KProbeMaxActive: -1,
		// },
		&probe.Probe{
			Name:            "setattr_tracker",
			Type:            probe.KProbeType,
			EntryFunc:       "trace_security_inode_setattr",
			EntryEvent:      "security_inode_setattr",
			ExitFunc:        "trace_ret_security_inode_setattr",
			ExitEvent:       "security_inode_setattr",
			KProbeMaxActive: -1,
			PerfMaps: []*probe.PerfMap{
				&probe.PerfMap{
					UserSpaceBufferLen:  1000,
					PerfOutputTableName: "setattr_events",
					DataHandler:         HandleSecurityInodeSetattr,
				},
			},
		},
	},
}

// FilterSetupFunc - Filter setup function
/*func FilterSetupFunc(options *pmodel.ProbeManagerOptions, em *emonitor.EventMonitor) error {
	options.RLock()
	defer options.RUnlock()
	var settingsFlag uint8
	if len(options.Filters.InPids) > 0 {
		if err := em.ApplyPidFilter(options, "pid_filter"); err != nil {
			return err
		}
		settingsFlag += 1 << 0
	} else if len(options.Filters.ExceptPids) > 0 {
		if err := em.ApplyPidFilter(options, "pid_filter"); err != nil {
			return err
		}
		settingsFlag += 1 << 1
	}
	if len(options.Filters.InPidns) > 0 {
		if err := em.ApplyPidnsFilter(options, "pidns_filter"); err != nil {
			return err
		}
		settingsFlag += 1 << 2
	} else if len(options.Filters.ExceptPidns) > 0 {
		if err := em.ApplyPidnsFilter(options, "pidns_filter"); err != nil {
			return err
		}
		settingsFlag += 1 << 3
	}
	if options.Filters.TTYOnly {
		settingsFlag += 1 << 4
	}

	// Apply filter settings
	settings := em.Tables["filter_settings"]
	if settings == nil {
		return fmt.Errorf("filter_settings BPF_HASH table doesn't exist")
	}
	// Setup filtering
	if err := settings.Set([]byte{1}, []byte{settingsFlag}); err != nil {
		return fmt.Errorf("couldn't set filter_settings: %v", err)
	}
	return nil
}*/

// HandleDentryEvent - Handles a dentry event
func HandleDentryEvent(data []byte, cache *probe.SafeCache, em *probe.GenericEventMonitor) {
	// Decode data from probe
	var eventRaw probe.DentryEventRaw
	err := binary.Read(bytes.NewBuffer(data), bcc.GetHostByteOrder(), &eventRaw)
	if err != nil {
		fmt.Printf("failed to decode received data (dentryEvent): %s\n", err)
		return
	}

	// Prepare event
	event := probe.DentryEvent{
		EventBase: probe.EventBase{
			EventType:        eventRaw.GetProbeEventType(),
			EventMonitorName: probe.FimMonitor,
			EventMonitorType: probe.EBPF,
			Timestamp:        probe.Manager.BootTime.Add(time.Duration(eventRaw.TimestampRaw) * time.Nanosecond),
		},
		DentryEventRaw: &eventRaw,
		TTYName:        C.GoString((*C.char)(unsafe.Pointer(&eventRaw.TTYNameRaw))),
	}
	event.SrcFilename, err = ResolveDentryPath(eventRaw.SrcPathnameKey, em)
	if err != nil {
		fmt.Printf("failed to resolve dentry path: %v", err)
	}

	switch event.EventType {
	case probe.FileHardLinkEventType, probe.FileRenameEventType:
		event.TargetFilename, err = ResolveDentryPath(eventRaw.TargetPathnameKey, em)
		if err != nil {
			fmt.Printf("failed to resolve dentry path: %v", err)
		}
	}

	// Dispatch dentry event
	probe.Manager.DispatchEvent(&event)
}

// HandleSecurityInodeSetattr - Handles a setattr update event
func HandleSecurityInodeSetattr(data []byte, cache *probe.SafeCache, em *probe.GenericEventMonitor) {
	// Parse data from probe
	var eventRaw probe.SetAttrRaw
	err := binary.Read(bytes.NewBuffer(data), bcc.GetHostByteOrder(), &eventRaw)
	if err != nil {
		fmt.Printf("failed to decode received data: %s\n", err)
		return
	}
	// Prepare event
	event := probe.SetAttrEvent{
		EventBase: probe.EventBase{
			EventType:        probe.FileSetAttrEventType,
			EventMonitorName: probe.FimMonitor,
			EventMonitorType: probe.EBPF,
			Timestamp:        probe.Manager.BootTime.Add(time.Duration(eventRaw.TimestampRaw) * time.Nanosecond),
		},
		SetAttrRaw: &eventRaw,
		TTYName:    C.GoString((*C.char)(unsafe.Pointer(&eventRaw.TTYNameRaw))),
		Atime:      time.Unix(eventRaw.AtimeRaw[0], eventRaw.AtimeRaw[1]),
		Mtime:      time.Unix(eventRaw.MtimeRaw[0], eventRaw.MtimeRaw[1]),
		Ctime:      time.Unix(eventRaw.CtimeRaw[0], eventRaw.CtimeRaw[1]),
	}
	event.Pathname, err = ResolveDentryPath(eventRaw.PathnameKey, em)
	if err != nil {
		fmt.Printf("failed to resolve dentry path (setAttr): %v", err)
	}

	// Filter and dispatch the event
	probe.Manager.DispatchEvent(&event)
}

type pathLeaf struct {
	ParentKey uint32
	Name      [255]byte
}

// ResolveDentryPath - Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func ResolveDentryPath(pathnameKey uint32, em *probe.GenericEventMonitor) (string, error) {
	// Don't resolve path if pathnameKey isn't valid
	if pathnameKey <= 0 {
		return "", fmt.Errorf("invalid pathname key %v", pathnameKey)
	}
	table := em.Tables["pathnames"]
	if table == nil {
		return "", fmt.Errorf("pathnames BPF_HASH table doesn't exist")
	}
	// Convert key into bytes
	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, pathnameKey)
	filename := ""
	done := false
	pathRaw := []byte{}
	var path pathLeaf
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
			err1 = fmt.Errorf("%v pathnames map deletion error: %v", em.GetMonitorName(), err2)
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
