package probe

import (
	ebpf "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
)

type EventHandler interface {
	HandleEvent(event interface{})
}

type Probe struct {
	*ebpf.Probe
	handler EventHandler
}

func NewProbe(handler EventHandler) (*Probe, error) {
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

	return p, nil
}

func (p *Probe) DispatchEvent(event interface{}) {
	p.handler.HandleEvent(event)
}
