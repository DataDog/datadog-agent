// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"time"

	libebpf "github.com/DataDog/ebpf"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// discarderRevisionSize array size used to store discarder revisions
	discarderRevisionSize = 4096
)

// Discarder represents a discarder which is basically the field that we know for sure
// that the value will be always rejected by the rules
type Discarder struct {
	Field eval.Field
}

type onDiscarderHandler func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error

var (
	allDiscarderHandlers = make(map[eval.EventType]onDiscarderHandler)
	// SupportedDiscarders lists all field which supports discarders
	SupportedDiscarders = make(map[eval.Field]bool)
)

var (
	dentryInvalidDiscarder = []interface{}{dentryPathKeyNotFound}
)

// InvalidDiscarders exposes list of values that are not discarders
var InvalidDiscarders = map[eval.Field][]interface{}{
	"open.filename":        dentryInvalidDiscarder,
	"unlink.filename":      dentryInvalidDiscarder,
	"chmod.filename":       dentryInvalidDiscarder,
	"chown.filename":       dentryInvalidDiscarder,
	"mkdir.filename":       dentryInvalidDiscarder,
	"rmdir.filename":       dentryInvalidDiscarder,
	"rename.old.filename":  dentryInvalidDiscarder,
	"rename.new.filename":  dentryInvalidDiscarder,
	"utimes.filename":      dentryInvalidDiscarder,
	"link.source.filename": dentryInvalidDiscarder,
	"link.target.filename": dentryInvalidDiscarder,
	"process.filename":     dentryInvalidDiscarder,
	"setxattr.filename":    dentryInvalidDiscarder,
	"removexattr.filename": dentryInvalidDiscarder,
}

// rearrange invalid discarders for fast lookup
func getInvalidDiscarders() map[eval.Field]map[interface{}]bool {
	invalidDiscarders := make(map[eval.Field]map[interface{}]bool)

	if InvalidDiscarders != nil {
		for field, values := range InvalidDiscarders {
			ivalues := invalidDiscarders[field]
			if ivalues == nil {
				ivalues = make(map[interface{}]bool)
				invalidDiscarders[field] = ivalues
			}
			for _, value := range values {
				ivalues[value] = true
			}
		}
	}

	return invalidDiscarders
}

type pidDiscarderParameters struct {
	EventType  model.EventType
	Timestamps [model.MaxEventRoundedUp]uint64
}

func (p *Probe) discardPID(eventType model.EventType, pid uint32) error {
	var params pidDiscarderParameters

	updateFlags := libebpf.UpdateExist
	if err := p.pidDiscarders.Lookup(pid, &params); err != nil {
		updateFlags = libebpf.UpdateAny
	}

	params.EventType |= 1 << (eventType - 1)
	return p.pidDiscarders.Update(pid, &params, updateFlags)
}

func (p *Probe) discardPIDWithTimeout(eventType model.EventType, pid uint32, timeout time.Duration) error {
	var params pidDiscarderParameters

	updateFlags := libebpf.UpdateExist
	if err := p.pidDiscarders.Lookup(pid, &params); err != nil {
		updateFlags = libebpf.UpdateAny
	}

	params.EventType |= 1 << (eventType - 1)
	params.Timestamps[eventType] = uint64(p.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now().Add(timeout)))

	return p.pidDiscarders.Update(pid, &params, updateFlags)
}

type inodeDiscarder struct {
	PathKey  PathKey
	Revision uint32
	Padding  uint32
}

type inodeDiscarderParameters struct {
	ParentMask model.EventType
	LeafMask   model.EventType
}

func (p *Probe) removeDiscarderInode(mountID uint32, inode uint64) {
	key := inodeDiscarder{
		PathKey: PathKey{
			MountID: mountID,
			Inode:   inode,
		},
	}
	_ = p.inodeDiscarders.Delete(&key)
}

func (p *Probe) discardInode(eventType model.EventType, mountID uint32, inode uint64, isLeaf bool) error {
	var params inodeDiscarderParameters
	key := inodeDiscarder{
		PathKey: PathKey{
			MountID: mountID,
			Inode:   inode,
		},
		Revision: p.getDiscarderRevision(mountID),
	}

	updateFlags := libebpf.UpdateExist
	if err := p.inodeDiscarders.Lookup(key, &params); err != nil {
		updateFlags = libebpf.UpdateAny
	}

	if isLeaf {
		params.LeafMask |= 1 << (eventType - 1)
	} else {
		params.ParentMask |= 1 << (eventType - 1)
	}

	return p.inodeDiscarders.Update(&key, &params, updateFlags)
}

func (p *Probe) discardParentInode(rs *rules.RuleSet, eventType model.EventType, field eval.Field, filename string, mountID uint32, inode uint64, pathID uint32) (bool, uint32, uint64, error) {
	isDiscarder, err := isParentPathDiscarder(rs, p.regexCache, eventType, field, filename)
	if !isDiscarder {
		return false, 0, 0, err
	}

	parentMountID, parentInode, err := p.resolvers.DentryResolver.GetParent(mountID, inode, pathID)
	if err != nil {
		return false, 0, 0, err
	}

	if err := p.discardInode(eventType, parentMountID, parentInode, false); err != nil {
		return false, 0, 0, err
	}

	return true, parentMountID, parentInode, nil
}

// function used to retrieve discarder information, *.filename, mountID, inode, file deleted
type inodeEventGetter = func(event *Event) (eval.Field, uint32, uint64, uint32, bool)

func filenameDiscarderWrapper(eventType model.EventType, handler onDiscarderHandler, getter inodeEventGetter) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
		field, mountID, inode, pathID, isDeleted := getter(event)

		if discarder.Field == field {
			value, err := event.GetFieldValue(field)
			if err != nil {
				return err
			}
			filename := value.(string)

			if filename == "" {
				return nil
			}

			if probe.IsInvalidDiscarder(field, filename) {
				return nil
			}

			isDiscarded, _, parentInode, err := probe.discardParentInode(rs, eventType, field, filename, mountID, inode, pathID)
			if !isDiscarded && !isDeleted {
				if _, ok := err.(*ErrInvalidKeyPath); !ok {
					log.Tracef("Apply `%s.filename` inode discarder for event `%s`, inode: %d", eventType, eventType, inode)

					// not able to discard the parent then only discard the filename
					err = probe.discardInode(eventType, mountID, inode, false)
				}
			} else {
				log.Tracef("Apply `%s.filename` parent inode discarder for event `%s` with value `%s`", eventType, eventType, filename)
			}

			if err != nil {
				err = errors.Wrapf(err, "unable to set inode discarders for `%s` for event `%s`, inode: %d", filename, eventType, parentInode)
			}

			return err
		}

		if handler != nil {
			return handler(rs, event, probe, discarder)
		}

		return nil
	}
}

func processDiscarderWrapper(eventType model.EventType, fnc onDiscarderHandler) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
		if discarder.Field == "process.filename" {
			log.Tracef("Apply process.filename discarder for event `%s`, inode: %d", eventType, event.Process.Inode)

			// discard by PID for long running process
			if err := probe.discardPID(eventType, event.Process.Pid); err != nil {
				return err
			}

			return probe.discardInode(eventType, event.Process.MountID, event.Process.Inode, true)
		}

		if fnc != nil {
			return fnc(rs, event, probe, discarder)
		}

		return nil
	}
}

func init() {
	SupportedDiscarders["process.filename"] = true

	allDiscarderHandlers["open"] = processDiscarderWrapper(model.FileOpenEventType,
		filenameDiscarderWrapper(model.FileOpenEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "open.filename", event.Open.MountID, event.Open.Inode, event.Open.PathID, false
			}))
	SupportedDiscarders["open.filename"] = true

	allDiscarderHandlers["mkdir"] = processDiscarderWrapper(model.FileMkdirEventType,
		filenameDiscarderWrapper(model.FileMkdirEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "mkdir.filename", event.Mkdir.MountID, event.Mkdir.Inode, event.Mkdir.PathID, false
			}))
	SupportedDiscarders["mkdir.filename"] = true

	allDiscarderHandlers["link"] = processDiscarderWrapper(model.FileLinkEventType, nil)

	allDiscarderHandlers["rename"] = processDiscarderWrapper(model.FileRenameEventType, nil)

	allDiscarderHandlers["unlink"] = processDiscarderWrapper(model.FileUnlinkEventType,
		filenameDiscarderWrapper(model.FileUnlinkEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "unlink.filename", event.Unlink.MountID, event.Unlink.Inode, event.Unlink.PathID, true
			}))
	SupportedDiscarders["unlink.filename"] = true

	allDiscarderHandlers["rmdir"] = processDiscarderWrapper(model.FileRmdirEventType,
		filenameDiscarderWrapper(model.FileRmdirEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "rmdir.filename", event.Rmdir.MountID, event.Rmdir.Inode, event.Rmdir.PathID, false
			}))
	SupportedDiscarders["rmdir.filename"] = true

	allDiscarderHandlers["chmod"] = processDiscarderWrapper(model.FileChmodEventType,
		filenameDiscarderWrapper(model.FileChmodEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "chmod.filename", event.Chmod.MountID, event.Chmod.Inode, event.Chmod.PathID, false
			}))
	SupportedDiscarders["chmod.filename"] = true

	allDiscarderHandlers["chown"] = processDiscarderWrapper(model.FileChownEventType,
		filenameDiscarderWrapper(model.FileChownEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "chown.filename", event.Chown.MountID, event.Chown.Inode, event.Chown.PathID, false
			}))
	SupportedDiscarders["chown.filename"] = true

	allDiscarderHandlers["utimes"] = processDiscarderWrapper(model.FileUtimeEventType,
		filenameDiscarderWrapper(model.FileUtimeEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "utimes.filename", event.Utimes.MountID, event.Utimes.Inode, event.Utimes.PathID, false
			}))
	SupportedDiscarders["utimes.filename"] = true

	allDiscarderHandlers["setxattr"] = processDiscarderWrapper(model.FileSetXAttrEventType,
		filenameDiscarderWrapper(model.FileSetXAttrEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "setxattr.filename", event.SetXAttr.MountID, event.SetXAttr.Inode, event.SetXAttr.PathID, false
			}))
	SupportedDiscarders["setxattr.filename"] = true

	allDiscarderHandlers["removexattr"] = processDiscarderWrapper(model.FileRemoveXAttrEventType,
		filenameDiscarderWrapper(model.FileRemoveXAttrEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "removexattr.filename", event.RemoveXAttr.MountID, event.RemoveXAttr.Inode, event.RemoveXAttr.PathID, false
			}))
	SupportedDiscarders["removexattr.filename"] = true
}
