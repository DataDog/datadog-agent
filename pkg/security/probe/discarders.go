// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
	"math"
	"math/rand"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	lib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DiscardInodeOp discards an inode
	DiscardInodeOp = iota + 1
	// DiscardPidOp discards a pid
	DiscardPidOp
)

const (
	// discarderRevisionSize array size used to store discarder revisions
	discarderRevisionSize = 4096

	// DiscardRetention time a discard is retained but not discarding. This avoid race for pending event is userspace
	// pipeline for already deleted file in kernel space.
	DiscardRetention = 5 * time.Second
)

var (
	// DiscarderConstants ebpf constants
	DiscarderConstants = []manager.ConstantEditor{
		{
			Name:  "discarder_retention",
			Value: uint64(DiscardRetention.Nanoseconds()),
		},
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
	dentryInvalidDiscarder = []interface{}{dentryPathKeyNotFound}
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

func (p *pidDiscarders) discard(eventType model.EventType, pid uint32) error {
	req := ERPCRequest{
		OP: DiscardPidOp,
	}

	offset := marshalDiscardHeader(&req, eventType, 0)
	model.ByteOrder.PutUint32(req.Data[offset:offset+4], pid)

	return p.erpc.Request(&req)
}

func (p *pidDiscarders) discardWithTimeout(eventType model.EventType, pid uint32, timeout int64) error {
	req := ERPCRequest{
		OP: DiscardPidOp,
	}

	offset := marshalDiscardHeader(&req, eventType, uint64(timeout))
	model.ByteOrder.PutUint32(req.Data[offset:offset+4], pid)

	return p.erpc.Request(&req)
}

func newPidDiscarders(m *lib.Map, erpc *ERPC) *pidDiscarders {
	return &pidDiscarders{Map: m, erpc: erpc}
}

type inodeDiscarder struct {
	PathKey  PathKey
	Revision uint32
	Padding  uint32
}

type inodeDiscarders struct {
	*lib.Map
	erpc           *ERPC
	revisions      *lib.Map
	revisionCache  [discarderRevisionSize]uint32
	dentryResolver *DentryResolver
	regexCache     *simplelru.LRU
}

func newInodeDiscarders(inodesMap, revisionsMap *lib.Map, erpc *ERPC, dentryResolver *DentryResolver) (*inodeDiscarders, error) {
	regexCache, err := simplelru.NewLRU(64, nil)
	if err != nil {
		return nil, err
	}

	return &inodeDiscarders{
		Map:            inodesMap,
		erpc:           erpc,
		revisions:      revisionsMap,
		dentryResolver: dentryResolver,
		regexCache:     regexCache,
	}, nil
}

func (id *inodeDiscarders) discardInode(eventType model.EventType, mountID uint32, inode uint64, isLeaf bool) error {
	req := ERPCRequest{
		OP: DiscardInodeOp,
	}

	var isLeafInt uint32
	if isLeaf {
		isLeafInt = 1
	}

	offset := marshalDiscardHeader(&req, eventType, 0)
	model.ByteOrder.PutUint64(req.Data[offset:offset+8], inode)
	model.ByteOrder.PutUint32(req.Data[offset+8:offset+12], mountID)
	model.ByteOrder.PutUint32(req.Data[offset+12:offset+16], isLeafInt)

	return id.erpc.Request(&req)
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

// Important should always be called after having checked that the file is not a discarder itself otherwise it can report incorrect
// parent discarder
func isParentPathDiscarder(rs *rules.RuleSet, regexCache *simplelru.LRU, eventType model.EventType, filenameField eval.Field, filename string) (bool, error) {
	dirname := filepath.Dir(filename)

	bucket := rs.GetBucket(eventType.String())
	if bucket == nil {
		return false, nil
	}

	basenameField := strings.Replace(filenameField, ".path", ".name", 1)

	event := NewEvent(nil, nil)
	if _, err := event.GetFieldType(filenameField); err != nil {
		return false, nil
	}

	if _, err := event.GetFieldType(basenameField); err != nil {
		return false, nil
	}

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
		if values := rule.GetFieldValues(filenameField); len(values) > 0 {
			for _, value := range values {
				if value.Type == eval.PatternValueType {
					if value.Regex.MatchString(dirname) {
						return false, nil
					}

					valueDir := path.Dir(value.Value.(string))
					var regexDir *regexp.Regexp
					if entry, found := regexCache.Get(valueDir); found {
						regexDir = entry.(*regexp.Regexp)
					} else {
						var err error
						regexDir, err = regexp.Compile(valueDir)
						if err != nil {
							return false, err
						}
						regexCache.Add(valueDir, regexDir)
					}

					if regexDir.MatchString(dirname) {
						return false, nil
					}
				} else {
					if strings.HasPrefix(value.Value.(string), dirname) {
						return false, nil
					}
				}
			}

			if err := event.SetFieldValue(filenameField, dirname); err != nil {
				return false, err
			}

			if isDiscarder, _ := rs.IsDiscarder(event, filenameField); isDiscarder {
				return true, nil
			}
		}

		// check basename
		if values := rule.GetFieldValues(basenameField); len(values) > 0 {
			if err := event.SetFieldValue(basenameField, path.Base(dirname)); err != nil {
				return false, err
			}

			if isDiscarder, _ := rs.IsDiscarder(event, basenameField); !isDiscarder {
				return false, nil
			}
		}
	}

	log.Tracef("`%s` discovered as parent discarder", dirname)

	return true, nil
}

func (id *inodeDiscarders) discardParentInode(rs *rules.RuleSet, eventType model.EventType, field eval.Field, filename string, mountID uint32, inode uint64, pathID uint32) (bool, uint32, uint64, error) {
	isDiscarder, err := isParentPathDiscarder(rs, id.regexCache, eventType, field, filename)
	if !isDiscarder {
		return false, 0, 0, err
	}

	parentMountID, parentInode, err := id.dentryResolver.GetParent(mountID, inode, pathID)
	if err != nil {
		return false, 0, 0, err
	}

	if err := id.discardInode(eventType, parentMountID, parentInode, false); err != nil {
		return false, 0, 0, err
	}

	return true, parentMountID, parentInode, nil
}

// function used to retrieve discarder information, *.file.path, mountID, inode, file deleted
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

			if isInvalidDiscarder(field, filename) {
				return nil
			}

			isDiscarded, _, parentInode, err := probe.inodeDiscarders.discardParentInode(rs, eventType, field, filename, mountID, inode, pathID)
			if !isDiscarded && !isDeleted {
				if _, ok := err.(*ErrInvalidKeyPath); !ok {
					log.Tracef("Apply `%s.file.path` inode discarder for event `%s`, inode: %d", eventType, eventType, inode)

					// not able to discard the parent then only discard the filename
					err = probe.inodeDiscarders.discardInode(eventType, mountID, inode, true)
				}
			} else {
				log.Tracef("Apply `%s.file.path` parent inode discarder for event `%s` with value `%s`", eventType, eventType, filename)
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

func processDiscarderWrapper(eventType model.EventType, fnc onDiscarderHandler) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
		if discarder.Field == "process.file.path" {
			log.Tracef("Apply process.file.path discarder for event `%s`, inode: %d, pid: %d", eventType, event.ProcessContext.FileFields.Inode, event.ProcessContext.Pid)

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
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "open.file.path", event.Open.File.MountID, event.Open.File.Inode, event.Open.File.PathID, false
			}))
	SupportedDiscarders["open.file.path"] = true

	allDiscarderHandlers["mkdir"] = processDiscarderWrapper(model.FileMkdirEventType,
		filenameDiscarderWrapper(model.FileMkdirEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "mkdir.file.path", event.Mkdir.File.MountID, event.Mkdir.File.Inode, event.Mkdir.File.PathID, false
			}))
	SupportedDiscarders["mkdir.file.path"] = true

	allDiscarderHandlers["link"] = processDiscarderWrapper(model.FileLinkEventType, nil)

	allDiscarderHandlers["rename"] = processDiscarderWrapper(model.FileRenameEventType, nil)

	allDiscarderHandlers["unlink"] = processDiscarderWrapper(model.FileUnlinkEventType,
		filenameDiscarderWrapper(model.FileUnlinkEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "unlink.file.path", event.Unlink.File.MountID, event.Unlink.File.Inode, event.Unlink.File.PathID, true
			}))
	SupportedDiscarders["unlink.file.path"] = true

	allDiscarderHandlers["rmdir"] = processDiscarderWrapper(model.FileRmdirEventType,
		filenameDiscarderWrapper(model.FileRmdirEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "rmdir.file.path", event.Rmdir.File.MountID, event.Rmdir.File.Inode, event.Rmdir.File.PathID, false
			}))
	SupportedDiscarders["rmdir.file.path"] = true

	allDiscarderHandlers["chmod"] = processDiscarderWrapper(model.FileChmodEventType,
		filenameDiscarderWrapper(model.FileChmodEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "chmod.file.path", event.Chmod.File.MountID, event.Chmod.File.Inode, event.Chmod.File.PathID, false
			}))
	SupportedDiscarders["chmod.file.path"] = true

	allDiscarderHandlers["chown"] = processDiscarderWrapper(model.FileChownEventType,
		filenameDiscarderWrapper(model.FileChownEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "chown.file.path", event.Chown.File.MountID, event.Chown.File.Inode, event.Chown.File.PathID, false
			}))
	SupportedDiscarders["chown.file.path"] = true

	allDiscarderHandlers["utimes"] = processDiscarderWrapper(model.FileUtimeEventType,
		filenameDiscarderWrapper(model.FileUtimeEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "utimes.file.path", event.Utimes.File.MountID, event.Utimes.File.Inode, event.Utimes.File.PathID, false
			}))
	SupportedDiscarders["utimes.file.path"] = true

	allDiscarderHandlers["setxattr"] = processDiscarderWrapper(model.FileSetXAttrEventType,
		filenameDiscarderWrapper(model.FileSetXAttrEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "setxattr.file.path", event.SetXAttr.File.MountID, event.SetXAttr.File.Inode, event.SetXAttr.File.PathID, false
			}))
	SupportedDiscarders["setxattr.file.path"] = true

	allDiscarderHandlers["removexattr"] = processDiscarderWrapper(model.FileRemoveXAttrEventType,
		filenameDiscarderWrapper(model.FileRemoveXAttrEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "removexattr.file.path", event.RemoveXAttr.File.MountID, event.RemoveXAttr.File.Inode, event.RemoveXAttr.File.PathID, false
			}))
	SupportedDiscarders["removexattr.file.path"] = true
}
