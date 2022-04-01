// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	"math"
	"math/rand"
	"path"
	"strings"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// discarderRevisionSize array size used to store discarder revisions
	discarderRevisionSize = 4096

	// DiscardRetention time a discard is retained but not discarding. This avoid race for pending event is userspace
	// pipeline for already deleted file in kernel space.
	DiscardRetention = 5 * time.Second

	// maxParentDiscarderDepth defines the maximum parent depth to find parent discarders
	// the eBPF part need to be adapted accordingly
	maxParentDiscarderDepth = 3

	// allEventTypes is a mask to match all the events
	allEventTypes = 0xffffffffffffffff
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

type onDiscarderHandler func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error

var (
	allDiscarderHandlers = make(map[eval.EventType]onDiscarderHandler)
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
	req  ERPCRequest
}

func (p *pidDiscarders) discard(eventType model.EventType, pid uint32) error {
	offset := marshalDiscardHeader(&p.req, eventType, 0)
	model.ByteOrder.PutUint32(p.req.Data[offset:offset+4], pid)

	return p.erpc.Request(&p.req)
}

func (p *pidDiscarders) discardWithTimeout(eventType model.EventType, pid uint32, timeout int64) error {
	offset := marshalDiscardHeader(&p.req, eventType, uint64(timeout))
	model.ByteOrder.PutUint32(p.req.Data[offset:offset+4], pid)

	return p.erpc.Request(&p.req)
}

func newPidDiscarders(m *lib.Map, erpc *ERPC) *pidDiscarders {
	return &pidDiscarders{Map: m, erpc: erpc, req: ERPCRequest{OP: DiscardPidOp}}
}

type inodeDiscarder struct {
	PathKey PathKey
	IsLeaf  uint32
	Padding uint32
}

// inodeDiscarders is used to issue eRPC discarder requests
type inodeDiscarders struct {
	*lib.Map
	erpc           *ERPC
	req            ERPCRequest
	revisions      *lib.Map
	revisionCache  [discarderRevisionSize]uint32
	dentryResolver *DentryResolver
	rs             *rules.RuleSet

	// parentDiscarderFncs holds parent discarder functions per depth
	parentDiscarderFncs [maxParentDiscarderDepth]map[eval.Field]func(dirname string) (bool, error)
}

func newInodeDiscarders(inodesMap, revisionsMap *lib.Map, erpc *ERPC, dentryResolver *DentryResolver) (*inodeDiscarders, error) {
	id := &inodeDiscarders{
		Map:            inodesMap,
		erpc:           erpc,
		req:            ERPCRequest{OP: DiscardInodeOp},
		revisions:      revisionsMap,
		dentryResolver: dentryResolver,
	}

	id.initParentDiscarderFncs()

	return id, nil
}

func (id *inodeDiscarders) discardInode(eventType model.EventType, mountID uint32, inode uint64, isLeaf bool) error {
	var isLeafInt uint32
	if isLeaf {
		isLeafInt = 1
	}

	offset := marshalDiscardHeader(&id.req, eventType, 0)
	model.ByteOrder.PutUint64(id.req.Data[offset:offset+8], inode)
	model.ByteOrder.PutUint32(id.req.Data[offset+8:offset+12], mountID)
	model.ByteOrder.PutUint32(id.req.Data[offset+12:offset+16], isLeafInt)

	return id.erpc.Request(&id.req)
}

// expireInodeDiscarder sends an eRPC request to expire a discarder
func (id *inodeDiscarders) expireInodeDiscarder(mountID uint32, inode uint64) error {
	model.ByteOrder.PutUint64(id.req.Data[0:8], inode)
	model.ByteOrder.PutUint32(id.req.Data[8:12], mountID)

	return id.erpc.Request(&id.req)
}

func (id *inodeDiscarders) setRevision(mountID uint32, revision uint32) {
	key := mountID % discarderRevisionSize
	id.revisionCache[key] = revision
}

func (id *inodeDiscarders) initRevision(mountEvent *model.MountEvent) {
	var revision uint32

	if mountEvent.IsOverlayFS() {
		revision = uint32(rand.Intn(math.MaxUint16) + 1)
	}

	key := mountEvent.MountID % discarderRevisionSize
	id.revisionCache[key] = revision

	if err := id.revisions.Put(ebpf.Uint32MapItem(key), ebpf.Uint32MapItem(revision)); err != nil {
		log.Errorf("unable to initialize discarder revisions: %s", err)
	}
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
				if value.Type == eval.PatternValueType {
					glob, ok := value.StringMatcher.(*eval.GlobStringMatcher)
					if !ok {
						return nil, errors.New("unexpected string matcher")
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

func (id *inodeDiscarders) discardParentInode(rs *rules.RuleSet, eventType model.EventType, field eval.Field, filename string, mountID uint32, inode uint64, pathID uint32) (bool, uint32, uint64, error) {
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

	if err := id.discardInode(eventType, mountID, inode, false); err != nil {
		return false, 0, 0, err
	}

	return true, mountID, inode, nil
}

// function used to retrieve discarder information, *.file.path, FileEvent, file deleted
type inodeEventGetter = func(event *Event) (eval.Field, *model.FileEvent, bool)

func filenameDiscarderWrapper(eventType model.EventType, handler onDiscarderHandler, getter inodeEventGetter) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
		field, fileEvent, isDeleted := getter(event)

		if fileEvent.PathResolutionError != nil {
			return fileEvent.PathResolutionError
		}
		mountID, inode, pathID := fileEvent.MountID, fileEvent.Inode, fileEvent.PathID

		if discarder.Field == field {
			value, err := event.GetFieldValue(field)
			if err != nil {
				return err
			}
			filename := value.(string)

			if filename == "" {
				return nil
			}

			if isInvalidDiscarder(field, filename) {
				return nil
			}

			isDiscarded, _, parentInode, err := probe.inodeDiscarders.discardParentInode(rs, eventType, field, filename, mountID, inode, pathID)
			if !isDiscarded && !isDeleted {
				if _, ok := err.(*ErrInvalidKeyPath); !ok {
					if !IsFakeInode(inode) {
						seclog.Tracef("Apply `%s.file.path` inode discarder for event `%s`, inode: %d(%s)", eventType, eventType, inode, filename)

						// not able to discard the parent then only discard the filename
						_ = probe.inodeDiscarders.discardInode(eventType, mountID, inode, true)
					}
				}
			} else if !isDeleted {
				seclog.Tracef("Apply `%s.file.path` parent inode discarder for event `%s`, inode: %d(%s)", eventType, eventType, parentInode, filename)
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

func processDiscarderWrapper(eventType model.EventType, fnc onDiscarderHandler) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
		if discarder.Field == "process.file.path" {
			seclog.Tracef("Apply process.file.path discarder for event `%s`, inode: %d, pid: %d", eventType, event.ProcessContext.FileFields.Inode, event.ProcessContext.Pid)

			// discard by PID for long running process
			if err := probe.pidDiscarders.discard(eventType, event.ProcessContext.Pid); err != nil {
				return err
			}

			return probe.inodeDiscarders.discardInode(eventType, event.ProcessContext.FileFields.MountID, event.ProcessContext.FileFields.Inode, true)
		}

		if fnc != nil {
			return fnc(rs, event, probe, discarder)
		}

		return nil
	}
}

var invalidDiscarders map[eval.Field]map[interface{}]bool

func init() {
	invalidDiscarders = createInvalidDiscardersCache()

	SupportedDiscarders["process.file.path"] = true

	allDiscarderHandlers["open"] = processDiscarderWrapper(model.FileOpenEventType,
		filenameDiscarderWrapper(model.FileOpenEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "open.file.path", &event.Open.File, false
			}))
	SupportedDiscarders["open.file.path"] = true

	allDiscarderHandlers["mkdir"] = processDiscarderWrapper(model.FileMkdirEventType,
		filenameDiscarderWrapper(model.FileMkdirEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "mkdir.file.path", &event.Mkdir.File, false
			}))
	SupportedDiscarders["mkdir.file.path"] = true

	allDiscarderHandlers["link"] = processDiscarderWrapper(model.FileLinkEventType, nil)

	allDiscarderHandlers["rename"] = processDiscarderWrapper(model.FileRenameEventType, nil)

	allDiscarderHandlers["unlink"] = processDiscarderWrapper(model.FileUnlinkEventType,
		filenameDiscarderWrapper(model.FileUnlinkEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "unlink.file.path", &event.Unlink.File, true
			}))
	SupportedDiscarders["unlink.file.path"] = true

	allDiscarderHandlers["rmdir"] = processDiscarderWrapper(model.FileRmdirEventType,
		filenameDiscarderWrapper(model.FileRmdirEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "rmdir.file.path", &event.Rmdir.File, false
			}))
	SupportedDiscarders["rmdir.file.path"] = true

	allDiscarderHandlers["chmod"] = processDiscarderWrapper(model.FileChmodEventType,
		filenameDiscarderWrapper(model.FileChmodEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "chmod.file.path", &event.Chmod.File, false
			}))
	SupportedDiscarders["chmod.file.path"] = true

	allDiscarderHandlers["chown"] = processDiscarderWrapper(model.FileChownEventType,
		filenameDiscarderWrapper(model.FileChownEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "chown.file.path", &event.Chown.File, false
			}))
	SupportedDiscarders["chown.file.path"] = true

	allDiscarderHandlers["utimes"] = processDiscarderWrapper(model.FileUtimesEventType,
		filenameDiscarderWrapper(model.FileUtimesEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "utimes.file.path", &event.Utimes.File, false
			}))
	SupportedDiscarders["utimes.file.path"] = true

	allDiscarderHandlers["setxattr"] = processDiscarderWrapper(model.FileSetXAttrEventType,
		filenameDiscarderWrapper(model.FileSetXAttrEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "setxattr.file.path", &event.SetXAttr.File, false
			}))
	SupportedDiscarders["setxattr.file.path"] = true

	allDiscarderHandlers["removexattr"] = processDiscarderWrapper(model.FileRemoveXAttrEventType,
		filenameDiscarderWrapper(model.FileRemoveXAttrEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "removexattr.file.path", &event.RemoveXAttr.File, false
			}))
	SupportedDiscarders["removexattr.file.path"] = true

	allDiscarderHandlers["bpf"] = processDiscarderWrapper(model.BPFEventType, nil)

	allDiscarderHandlers["mmap"] = processDiscarderWrapper(model.MMapEventType,
		filenameDiscarderWrapper(model.MMapEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "mmap.file.path", &event.MMap.File, false
			}))
	SupportedDiscarders["mmap.file.path"] = true

	allDiscarderHandlers["splice"] = processDiscarderWrapper(model.SpliceEventType,
		filenameDiscarderWrapper(model.SpliceEventType, nil,
			func(event *Event) (eval.Field, *model.FileEvent, bool) {
				return "splice.file.path", &event.Splice.File, false
			}))
	SupportedDiscarders["splice.file.path"] = true

	allDiscarderHandlers["mprotect"] = processDiscarderWrapper(model.MProtectEventType, nil)
	allDiscarderHandlers["ptrace"] = processDiscarderWrapper(model.PTraceEventType, nil)
	allDiscarderHandlers["load_module"] = processDiscarderWrapper(model.LoadModuleEventType, nil)
	allDiscarderHandlers["unload_module"] = processDiscarderWrapper(model.UnloadModuleEventType, nil)
	allDiscarderHandlers["signal"] = processDiscarderWrapper(model.SignalEventType, nil)
}
