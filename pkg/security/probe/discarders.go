// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"errors"
	"fmt"
	"math"
	"path"
	"strings"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

const (
	// DiscardRetention time a discard is retained but not discarding. This avoid race for pending event is userspace
	// pipeline for already deleted file in kernel space.
	DiscardRetention = 5 * time.Second

	// maxParentDiscarderDepth defines the maximum parent depth to find parent discarders
	// the eBPF part need to be adapted accordingly
	maxParentDiscarderDepth = 3

	// allEventTypes is a mask to match all the events
	allEventTypes = math.MaxUint32

	// inode/mountid that won't be resubmitted
	maxRecentlyAddedCacheSize = uint64(64)

	// Map names for discarder stats. Discarder stats includes counts of discarders added and events discarded. Look up "multiple buffering" for more details about why there's two buffers.
	frontBufferDiscarderStatsMapName = "discarder_stats_fb"
	backBufferDiscarderStatsMapName  = "discarder_stats_bb"
)

var (
	// DiscarderConstants ebpf constants
	DiscarderConstants = []manager.ConstantEditor{
		{
			Name:  "discarder_retention",
			Value: uint64(DiscardRetention.Nanoseconds()),
		},
		/*{
			Name:  "max_discarder_depth",
			Value: uint64(maxParentDiscarderDepth),
		},*/
	}

	// recentlyAddedTimeout do not add twice the same discarder in 2sec
	recentlyAddedTimeout = uint64(2 * time.Second.Nanoseconds())
)

// Discarder represents a discarder which is basically the field that we know for sure
// that the value will be always rejected by the rules
type Discarder struct {
	Field eval.Field
}

// ErrDiscarderNotSupported is returned when trying to discover a discarder on a field that doesn't support them
type ErrDiscarderNotSupported struct {
	Field string
}

func (e ErrDiscarderNotSupported) Error() string {
	return fmt.Sprintf("discarder not supported for `%s`", e.Field)
}

type onDiscarderHandler func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) (bool, error)

var (
	allDiscarderHandlers = make(map[eval.EventType][]onDiscarderHandler)
	// SupportedDiscarders lists all field which supports discarders
	SupportedDiscarders = make(map[eval.Field]bool)
)

var (
	dentryInvalidDiscarder = []interface{}{""}
)

// InvalidDiscarders exposes list of values that are not discarders
var InvalidDiscarders = map[eval.Field][]interface{}{
	"open.file.path":               dentryInvalidDiscarder,
	"unlink.file.path":             dentryInvalidDiscarder,
	"chmod.file.path":              dentryInvalidDiscarder,
	"chown.file.path":              dentryInvalidDiscarder,
	"mkdir.file.path":              dentryInvalidDiscarder,
	"rmdir.file.path":              dentryInvalidDiscarder,
	"rename.file.path":             dentryInvalidDiscarder,
	"rename.file.destination.path": dentryInvalidDiscarder,
	"utimes.file.path":             dentryInvalidDiscarder,
	"link.file.path":               dentryInvalidDiscarder,
	"link.file.destination.path":   dentryInvalidDiscarder,
	"process.file.path":            dentryInvalidDiscarder,
	"setxattr.file.path":           dentryInvalidDiscarder,
	"removexattr.file.path":        dentryInvalidDiscarder,
}

func marshalDiscardHeader(req *ERPCRequest, eventType model.EventType, timeout uint64) int {
	model.ByteOrder.PutUint64(req.Data[0:8], uint64(eventType))
	model.ByteOrder.PutUint64(req.Data[8:16], timeout)

	return 16
}

type pidDiscarders struct {
	*lib.Map
	erpc *ERPC
}

func (p *pidDiscarders) discardPid(req *ERPCRequest, eventType model.EventType, pid uint32) error {
	req.OP = DiscardPidOp
	offset := marshalDiscardHeader(req, eventType, 0)
	model.ByteOrder.PutUint32(req.Data[offset:offset+4], pid)

	return p.erpc.Request(req)
}

func (p *pidDiscarders) discardWithTimeout(req *ERPCRequest, eventType model.EventType, pid uint32, timeout int64) error {
	req.OP = DiscardPidOp
	offset := marshalDiscardHeader(req, eventType, uint64(timeout))
	model.ByteOrder.PutUint32(req.Data[offset:offset+4], pid)

	return p.erpc.Request(req)
}

// expirePidDiscarder sends an eRPC request to expire a discarder
func (p *pidDiscarders) expirePidDiscarder(req *ERPCRequest, pid uint32) error {
	req.OP = ExpirePidDiscarderOp
	model.ByteOrder.PutUint32(req.Data[0:4], pid)

	return p.erpc.Request(req)
}

func newPidDiscarders(m *lib.Map, erpc *ERPC) *pidDiscarders {
	return &pidDiscarders{Map: m, erpc: erpc}
}

type inodeDiscarderMapEntry struct {
	PathKey PathKey
	IsLeaf  uint32
	Padding uint32
}

type inodeDiscarderEntry struct {
	Inode     uint64
	MountID   uint32
	Timestamp uint64
}

type inodeDiscarderParams struct {
	DiscarderParams discarderParams
	Revision        uint32
}

type pidDiscarderParams struct {
	DiscarderParams discarderParams
}

type discarderParams struct {
	EventMask  uint64
	Timestamps [model.LastDiscarderEventType - model.FirstDiscarderEventType]uint64
	ExpireAt   uint64
	IsRetained uint32
}

type discarderStats struct {
	DiscardersAdded uint64
	EventDiscarded  uint64
}

func recentlyAddedIndex(mountID uint32, inode uint64) uint64 {
	return (uint64(mountID)<<32 | inode) % maxRecentlyAddedCacheSize
}

// inodeDiscarders is used to issue eRPC discarder requests
type inodeDiscarders struct {
	*lib.Map
	erpc           *ERPC
	dentryResolver *DentryResolver
	rs             *rules.RuleSet

	// parentDiscarderFncs holds parent discarder functions per depth
	parentDiscarderFncs [maxParentDiscarderDepth]map[eval.Field]func(dirname string) (bool, error)

	recentlyAddedEntries [maxRecentlyAddedCacheSize]inodeDiscarderEntry
}

func newInodeDiscarders(inodesMap *lib.Map, erpc *ERPC, dentryResolver *DentryResolver) *inodeDiscarders {
	id := &inodeDiscarders{
		Map:            inodesMap,
		erpc:           erpc,
		dentryResolver: dentryResolver,
	}

	id.initParentDiscarderFncs()

	return id
}

func (id *inodeDiscarders) isRecentlyAdded(mountID uint32, inode uint64, timestamp uint64) bool {
	entry := id.recentlyAddedEntries[recentlyAddedIndex(mountID, inode)]

	var delta uint64
	if timestamp > entry.Timestamp {
		delta = timestamp - entry.Timestamp
	} else {
		delta = entry.Timestamp - timestamp
	}

	return entry.MountID == mountID && entry.Inode == inode && delta < recentlyAddedTimeout
}

func (id *inodeDiscarders) recentlyAdded(mountID uint32, inode uint64, timestamp uint64) {
	entry := &id.recentlyAddedEntries[recentlyAddedIndex(mountID, inode)]
	entry.MountID = mountID
	entry.Inode = inode
	entry.Timestamp = timestamp
}

func (id *inodeDiscarders) discardInode(req *ERPCRequest, eventType model.EventType, mountID uint32, inode uint64, isLeaf bool) error {
	var isLeafInt uint32
	if isLeaf {
		isLeafInt = 1
	}

	req.OP = DiscardInodeOp

	offset := marshalDiscardHeader(req, eventType, 0)
	model.ByteOrder.PutUint64(req.Data[offset:offset+8], inode)
	model.ByteOrder.PutUint32(req.Data[offset+8:offset+12], mountID)
	model.ByteOrder.PutUint32(req.Data[offset+12:offset+16], isLeafInt)

	return id.erpc.Request(req)
}

// expireInodeDiscarder sends an eRPC request to expire a discarder
func (id *inodeDiscarders) expireInodeDiscarder(req *ERPCRequest, mountID uint32, inode uint64) error {
	req.OP = ExpireInodeDiscarderOp
	model.ByteOrder.PutUint64(req.Data[0:8], inode)
	model.ByteOrder.PutUint32(req.Data[8:12], mountID)

	return id.erpc.Request(req)
}

var (
	discarderEvent = NewEvent(nil, nil, nil)
)

// use a faster version of path.Dir which adds some sanity checks not required here
func dirname(filename string) string {
	if len(filename) == 0 {
		return "/"
	}

	i := len(filename) - 1
	for i >= 0 && filename[i] != '/' {
		i--
	}

	if filename == "/" {
		return filename
	}

	if i <= 0 {
		return "/"
	}

	return filename[:i]
}

func getParent(filename string, depth int) string {
	for ; depth > 0; depth-- {
		filename = dirname(filename)
	}

	return filename
}

func (id *inodeDiscarders) getParentDiscarderFnc(rs *rules.RuleSet, eventType model.EventType, field eval.Field, depth int) (func(dirname string) (bool, error), error) {
	fnc, exists := id.parentDiscarderFncs[depth-1][field]
	if exists {
		return fnc, nil
	}

	bucket := rs.GetBucket(eventType.String())
	if bucket == nil {
		return nil, nil
	}

	if _, err := discarderEvent.GetFieldType(field); err != nil {
		return nil, err
	}

	if !strings.HasSuffix(field, model.PathSuffix) {
		return nil, errors.New("path suffix not found")
	}

	basenameField := strings.Replace(field, model.PathSuffix, model.NameSuffix, 1)
	if _, err := discarderEvent.GetFieldType(basenameField); err != nil {
		return nil, err
	}

	var valueFnc func(dirname string) (bool, bool, error)
	var valueFncs []func(dirname string) (bool, bool, error)

	for _, rule := range bucket.GetRules() {
		// ensure we don't push parent discarder if there is another rule relying on the parent path

		// first case: rule contains a filename field
		// ex: rule		open.file.path == "/etc/passwd"
		//     discarder /etc/fstab
		// /etc/fstab is a discarder but not the parent

		// second case: rule doesn't contain a filename field but a basename field
		// ex: rule	 	open.file.name == "conf.d"
		//     discarder /etc/conf.d/httpd.conf
		// /etc/conf.d/httpd.conf is a discarder but not the parent

		// check filename
		if values := rule.GetFieldValues(field); len(values) > 0 {
			for _, value := range values {
				if value.Type == eval.GlobValueType {
					glob, err := eval.NewGlob(value.Value.(string), false)
					if err != nil {
						return nil, fmt.Errorf("unexpected glob `%v`: %w", value.Value, err)
					}

					valueFnc = func(dirname string) (bool, bool, error) {
						return !glob.Contains(dirname), false, nil
					}
				} else if value.Type == eval.ScalarValueType {
					str := value.Value.(string)
					valueFnc = func(dirname string) (bool, bool, error) {
						return !strings.HasPrefix(str, dirname), false, nil
					}
				} else {
					// regex are not currently supported on path, see ValidateFields
					valueFnc = func(dirname string) (bool, bool, error) {
						return false, false, nil
					}
				}

				valueFncs = append(valueFncs, valueFnc)
			}
		}

		// check basename
		if values := rule.GetFieldValues(basenameField); len(values) > 0 {
			valueFnc = func(dirname string) (bool, bool, error) {
				if err := discarderEvent.SetFieldValue(basenameField, path.Base(dirname)); err != nil {
					return false, false, err
				}

				if isDiscarder, _ := rs.IsDiscarder(discarderEvent, basenameField); !isDiscarder {
					return false, true, nil
				}

				return true, true, nil
			}
			valueFncs = append(valueFncs, valueFnc)
		}
	}

	fnc = func(dirname string) (bool, error) {
		var result, altered bool
		var err error

		defer func() {
			if altered {
				*discarderEvent = eventZero
			}
		}()

		for _, fnc := range valueFncs {
			result, altered, err = fnc(dirname)
			if !result {
				return false, err
			}
		}

		return len(valueFncs) > 0, nil
	}
	id.parentDiscarderFncs[depth-1][field] = fnc

	return fnc, nil
}

func (id *inodeDiscarders) initParentDiscarderFncs() {
	for i := range id.parentDiscarderFncs {
		id.parentDiscarderFncs[i] = make(map[eval.Field]func(dirname string) (bool, error))
	}
}

// onRuleSetChanged if the ruleset changed we need to flush all the previous functions
func (id *inodeDiscarders) onRuleSetChanged(rs *rules.RuleSet) {
	id.initParentDiscarderFncs()
	id.rs = rs
}

func (id *inodeDiscarders) isParentPathDiscarder(rs *rules.RuleSet, eventType model.EventType, field eval.Field, filename string, depth int) (bool, error) {
	if id.rs != rs {
		id.onRuleSetChanged(rs)
	}

	fnc, err := id.getParentDiscarderFnc(rs, eventType, field, depth)
	if fnc == nil || err != nil {
		return false, err
	}

	dirname := getParent(filename, depth)
	if dirname == "/" {
		// never discard /
		return false, nil
	}

	found, err := fnc(dirname)
	if !found || err != nil {
		return false, err
	}

	seclog.Tracef("`%s` discovered as parent discarder for `%s`", dirname, field)

	return true, nil
}

func (id *inodeDiscarders) discardParentInode(req *ERPCRequest, rs *rules.RuleSet, eventType model.EventType, field eval.Field, filename string, mountID uint32, inode uint64, pathID uint32, timestamp uint64) (bool, uint32, uint64, error) {
	var discarderDepth int
	var isDiscarder bool
	var err error

	for depth := maxParentDiscarderDepth; depth > 0; depth-- {
		if isDiscarder, err = id.isParentPathDiscarder(rs, eventType, field, filename, depth); isDiscarder {
			discarderDepth = depth
			break
		}
	}

	if err != nil || discarderDepth == 0 {
		return false, 0, 0, err
	}

	for i := 0; i < discarderDepth; i++ {
		parentMountID, parentInode, err := id.dentryResolver.GetParent(mountID, inode, pathID)
		if err != nil || IsFakeInode(parentInode) {
			if i == 0 {
				return false, 0, 0, err
			}
			break
		}
		mountID, inode = parentMountID, parentInode
	}

	// do not insert multiple time the same discarder
	if id.isRecentlyAdded(mountID, inode, timestamp) {
		return false, 0, 0, nil
	}

	if err := id.discardInode(req, eventType, mountID, inode, false); err != nil {
		return false, 0, 0, err
	}

	id.recentlyAdded(mountID, inode, timestamp)

	return true, mountID, inode, nil
}

// function used to retrieve discarder information, *.file.path, FileEvent, file deleted
type inodeEventGetter = func(event *Event) (eval.Field, *model.FileEvent, bool)

func filenameDiscarderWrapper(eventType model.EventType, getter inodeEventGetter) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) (bool, error) {
		field, fileEvent, isDeleted := getter(event)

		if fileEvent.PathResolutionError != nil {
			return false, fileEvent.PathResolutionError
		}
		mountID, inode, pathID := fileEvent.MountID, fileEvent.Inode, fileEvent.PathID

		if discarder.Field == field {
			value, err := event.GetFieldValue(field)
			if err != nil {
				return false, err
			}
			filename := value.(string)

			if filename == "" {
				return false, nil
			}

			if isInvalidDiscarder(field, filename) {
				return false, nil
			}

			isDiscarded, _, parentInode, err := probe.inodeDiscarders.discardParentInode(probe.erpcRequest, rs, eventType, field, filename, mountID, inode, pathID, event.TimestampRaw)
			if !isDiscarded && !isDeleted {
				if _, ok := err.(*ErrInvalidKeyPath); !ok {
					if !IsFakeInode(inode) {
						seclog.Tracef("Apply `%s.file.path` inode discarder for event `%s`, inode: %d(%s)", eventType, eventType, inode, filename)

						// not able to discard the parent then only discard the filename
						_ = probe.inodeDiscarders.discardInode(probe.erpcRequest, eventType, mountID, inode, true)
					}
				}
			} else if !isDeleted {
				seclog.Tracef("Apply `%s.file.path` parent inode discarder for event `%s`, inode: %d(%s)", eventType, eventType, parentInode, filename)
			}

			if err != nil {
				err = fmt.Errorf("unable to set inode discarders for `%s` for event `%s`, inode: %d: %w", filename, eventType, parentInode, err)
			}

			return true, err
		}

		return false, nil
	}
}

// isInvalidDiscarder returns whether the given value is a valid discarder for the given field
func isInvalidDiscarder(field eval.Field, value interface{}) bool {
	values, exists := invalidDiscarders[field]
	if !exists {
		return false
	}

	return values[value]
}

// rearrange invalid discarders for fast lookup
func createInvalidDiscardersCache() map[eval.Field]map[interface{}]bool {
	invalidDiscarders := make(map[eval.Field]map[interface{}]bool)

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

	return invalidDiscarders
}

func processDiscarderWrapper(eventType model.EventType) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) (bool, error) {
		if discarder.Field == "process.file.path" {
			seclog.Tracef("Apply process.file.path discarder for event `%s`, inode: %d, pid: %d", eventType, event.ProcessContext.FileEvent.Inode, event.ProcessContext.Pid)

			if err := probe.pidDiscarders.discardPid(probe.erpcRequest, eventType, event.ProcessContext.Pid); err != nil {
				return false, err
			}

			return true, nil
		}

		return false, nil
	}
}

var invalidDiscarders map[eval.Field]map[interface{}]bool

func init() {
	invalidDiscarders = createInvalidDiscardersCache()

	SupportedDiscarders["process.file.path"] = true

	allDiscarderHandlers["open"] = append(allDiscarderHandlers["open"], processDiscarderWrapper(model.FileOpenEventType))
	allDiscarderHandlers["open"] = append(allDiscarderHandlers["open"], filenameDiscarderWrapper(model.FileOpenEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "open.file.path", &event.Open.File, false
		}))
	SupportedDiscarders["open.file.path"] = true

	allDiscarderHandlers["mkdir"] = append(allDiscarderHandlers["mkdir"], processDiscarderWrapper(model.FileMkdirEventType))
	allDiscarderHandlers["mkdir"] = append(allDiscarderHandlers["mkdir"], filenameDiscarderWrapper(model.FileMkdirEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "mkdir.file.path", &event.Mkdir.File, false
		}))
	SupportedDiscarders["mkdir.file.path"] = true

	allDiscarderHandlers["link"] = []onDiscarderHandler{processDiscarderWrapper(model.FileLinkEventType)}

	allDiscarderHandlers["rename"] = []onDiscarderHandler{processDiscarderWrapper(model.FileRenameEventType)}

	allDiscarderHandlers["unlink"] = append(allDiscarderHandlers["unlink"], processDiscarderWrapper(model.FileUnlinkEventType))
	allDiscarderHandlers["unlink"] = append(allDiscarderHandlers["unlink"], filenameDiscarderWrapper(model.FileUnlinkEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "unlink.file.path", &event.Unlink.File, true
		}))
	SupportedDiscarders["unlink.file.path"] = true

	allDiscarderHandlers["rmdir"] = append(allDiscarderHandlers["rmdir"], processDiscarderWrapper(model.FileRmdirEventType))
	allDiscarderHandlers["rmdir"] = append(allDiscarderHandlers["rmdir"], filenameDiscarderWrapper(model.FileRmdirEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "rmdir.file.path", &event.Rmdir.File, false
		}))
	SupportedDiscarders["rmdir.file.path"] = true

	allDiscarderHandlers["chmod"] = append(allDiscarderHandlers["chmod"], processDiscarderWrapper(model.FileChmodEventType))
	allDiscarderHandlers["chmod"] = append(allDiscarderHandlers["chmod"], filenameDiscarderWrapper(model.FileChmodEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "chmod.file.path", &event.Chmod.File, false
		}))
	SupportedDiscarders["chmod.file.path"] = true

	allDiscarderHandlers["chown"] = append(allDiscarderHandlers["chown"], processDiscarderWrapper(model.FileChownEventType))
	allDiscarderHandlers["chown"] = append(allDiscarderHandlers["chown"], filenameDiscarderWrapper(model.FileChownEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "chown.file.path", &event.Chown.File, false
		}))
	SupportedDiscarders["chown.file.path"] = true

	allDiscarderHandlers["utimes"] = append(allDiscarderHandlers["utimes"], processDiscarderWrapper(model.FileUtimesEventType))
	allDiscarderHandlers["utimes"] = append(allDiscarderHandlers["utimes"], filenameDiscarderWrapper(model.FileUtimesEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "utimes.file.path", &event.Utimes.File, false
		}))
	SupportedDiscarders["utimes.file.path"] = true

	allDiscarderHandlers["setxattr"] = append(allDiscarderHandlers["setxattr"], processDiscarderWrapper(model.FileSetXAttrEventType))
	allDiscarderHandlers["setxattr"] = append(allDiscarderHandlers["setxattr"], filenameDiscarderWrapper(model.FileSetXAttrEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "setxattr.file.path", &event.SetXAttr.File, false
		}))
	SupportedDiscarders["setxattr.file.path"] = true

	allDiscarderHandlers["removexattr"] = append(allDiscarderHandlers["removexattr"], processDiscarderWrapper(model.FileRemoveXAttrEventType))
	allDiscarderHandlers["removexattr"] = append(allDiscarderHandlers["removexattr"], filenameDiscarderWrapper(model.FileRemoveXAttrEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "removexattr.file.path", &event.RemoveXAttr.File, false
		}))
	SupportedDiscarders["removexattr.file.path"] = true

	allDiscarderHandlers["bpf"] = []onDiscarderHandler{processDiscarderWrapper(model.BPFEventType)}

	allDiscarderHandlers["mmap"] = append(allDiscarderHandlers["mmap"], processDiscarderWrapper(model.MMapEventType))
	allDiscarderHandlers["mmap"] = append(allDiscarderHandlers["mmap"], filenameDiscarderWrapper(model.MMapEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "mmap.file.path", &event.MMap.File, false
		}))
	SupportedDiscarders["mmap.file.path"] = true

	allDiscarderHandlers["splice"] = append(allDiscarderHandlers["splice"], processDiscarderWrapper(model.SpliceEventType))
	allDiscarderHandlers["splice"] = append(allDiscarderHandlers["splice"], filenameDiscarderWrapper(model.SpliceEventType,
		func(event *Event) (eval.Field, *model.FileEvent, bool) {
			return "splice.file.path", &event.Splice.File, false
		}))
	SupportedDiscarders["splice.file.path"] = true

	allDiscarderHandlers["mprotect"] = []onDiscarderHandler{processDiscarderWrapper(model.MProtectEventType)}
	allDiscarderHandlers["ptrace"] = []onDiscarderHandler{processDiscarderWrapper(model.PTraceEventType)}
	allDiscarderHandlers["load_module"] = []onDiscarderHandler{processDiscarderWrapper(model.LoadModuleEventType)}
	allDiscarderHandlers["unload_module"] = []onDiscarderHandler{processDiscarderWrapper(model.UnloadModuleEventType)}
	allDiscarderHandlers["signal"] = []onDiscarderHandler{processDiscarderWrapper(model.SignalEventType)}
	allDiscarderHandlers["bind"] = []onDiscarderHandler{processDiscarderWrapper(model.BindEventType)}
}
