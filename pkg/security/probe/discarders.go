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
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
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
)

var (
	// DiscarderConstants ebpf constants
	DiscarderConstants = []manager.ConstantEditor{
		{
			Name:  "discarder_retention",
			Value: uint64(DiscardRetention.Nanoseconds()),
		},
	}

	// recentlyAddedTimeout do not add twice the same discarder in 2sec
	recentlyAddedTimeout = uint64(2 * time.Second.Nanoseconds())
)

// Discarder represents a discarder which is basically the field that we know for sure
// that the value will be always rejected by the rules
type Discarder struct {
	Field eval.Field
}

// DiscarderStats is used to collect kernel space metrics about discarders
type DiscarderStats struct {
	DiscarderAdded uint64 `yaml:"discarder_added"`
	EventDiscarded uint64 `yaml:"event_discarded"`
}

// ErrDiscarderNotSupported is returned when trying to discover a discarder on a field that doesn't support them
type ErrDiscarderNotSupported struct {
	Field string
}

func (e ErrDiscarderNotSupported) Error() string {
	return fmt.Sprintf("discarder not supported for `%s`", e.Field)
}

type onDiscarderHandler func(rs *rules.RuleSet, event *model.Event, probe *Probe, discarder Discarder) (bool, error)

var (
	allDiscarderHandlers = make(map[eval.EventType][]onDiscarderHandler)
	// SupportedDiscarders lists all field which supports discarders
	SupportedDiscarders = make(map[eval.Field]bool)
)

var (
	dentryInvalidDiscarder = []interface{}{""}
	eventZeroDiscarder     = &model.Event{
		FieldHandlers:    &model.DefaultFieldHandlers{},
		ContainerContext: &model.ContainerContext{},
	}
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

// bumpDiscardersRevision sends an eRPC request to bump the discarders revisionr
func bumpDiscardersRevision(e *erpc.ERPC) error {
	var req erpc.ERPCRequest
	req.OP = erpc.BumpDiscardersRevision
	return e.Request(&req)
}

func marshalDiscardHeader(req *erpc.ERPCRequest, eventType model.EventType, timeout uint64) int {
	model.ByteOrder.PutUint64(req.Data[0:8], uint64(eventType))
	model.ByteOrder.PutUint64(req.Data[8:16], timeout)

	return 16
}

type pidDiscarders struct {
	erpc *erpc.ERPC
}

func (p *pidDiscarders) discardWithTimeout(req *erpc.ERPCRequest, eventType model.EventType, pid uint32, timeout int64) error {
	req.OP = erpc.DiscardPidOp
	offset := marshalDiscardHeader(req, eventType, uint64(timeout))
	model.ByteOrder.PutUint32(req.Data[offset:offset+4], pid)

	return p.erpc.Request(req)
}

func newPidDiscarders(erpc *erpc.ERPC) *pidDiscarders {
	return &pidDiscarders{erpc: erpc}
}

// InodeDiscarderMapEntry describes a map entry
type InodeDiscarderMapEntry struct {
	PathKey model.PathKey
	IsLeaf  uint32
	Padding uint32
}

// InodeDiscarderEntry describes a map entry
type InodeDiscarderEntry struct {
	Inode     uint64
	MountID   uint32
	Timestamp uint64
}

// InodeDiscarderParams describes a map value
type InodeDiscarderParams struct {
	DiscarderParams `yaml:"params"`
	Revision        uint32
}

// PidDiscarderParams describes a map value
type PidDiscarderParams struct {
	DiscarderParams `yaml:"params"`
}

// DiscarderParams describes a map value
type DiscarderParams struct {
	EventMask  uint64                                                               `yaml:"event_mask"`
	Timestamps [model.LastDiscarderEventType - model.FirstDiscarderEventType]uint64 `yaml:"-"`
	ExpireAt   uint64                                                               `yaml:"expire_at"`
	IsRetained uint32                                                               `yaml:"is_retained"`
	Revision   uint32
}

func recentlyAddedIndex(mountID uint32, inode uint64) uint64 {
	return (uint64(mountID)<<32 | inode) % maxRecentlyAddedCacheSize
}

// inodeDiscarders is used to issue eRPC discarder requests
type inodeDiscarders struct {
	erpc           *erpc.ERPC
	dentryResolver *dentry.Resolver
	rs             *rules.RuleSet
	discarderEvent *model.Event
	evalCtx        *eval.Context

	// parentDiscarderFncs holds parent discarder functions per depth
	parentDiscarderFncs [maxParentDiscarderDepth]map[eval.Field]func(dirname string) (bool, error)

	recentlyAddedEntries [maxRecentlyAddedCacheSize]InodeDiscarderEntry
}

func newInodeDiscarders(erpc *erpc.ERPC, dentryResolver *dentry.Resolver) *inodeDiscarders {
	event := *eventZeroDiscarder

	ctx := eval.NewContext(&event)

	id := &inodeDiscarders{
		erpc:           erpc,
		dentryResolver: dentryResolver,
		discarderEvent: &event,
		evalCtx:        ctx,
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

func (id *inodeDiscarders) discardInode(req *erpc.ERPCRequest, eventType model.EventType, mountID uint32, inode uint64, isLeaf bool) error {
	var isLeafInt uint32
	if isLeaf {
		isLeafInt = 1
	}

	req.OP = erpc.DiscardInodeOp

	offset := marshalDiscardHeader(req, eventType, 0)
	model.ByteOrder.PutUint64(req.Data[offset:offset+8], inode)
	model.ByteOrder.PutUint32(req.Data[offset+8:offset+12], mountID)
	model.ByteOrder.PutUint32(req.Data[offset+12:offset+16], isLeafInt)

	return id.erpc.Request(req)
}

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

	if _, err := id.discarderEvent.GetFieldType(field); err != nil {
		return nil, err
	}

	if !strings.HasSuffix(field, model.PathSuffix) {
		return nil, errors.New("path suffix not found")
	}

	basenameField := strings.Replace(field, model.PathSuffix, model.NameSuffix, 1)
	if _, err := id.discarderEvent.GetFieldType(basenameField); err != nil {
		return nil, err
	}

	var basenameRules []*rules.Rule

	var isDiscarderFnc func(dirname string) (bool, bool, error)
	var isDiscarderFncs []func(dirname string) (bool, bool, error)

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

					isDiscarderFnc = func(dirname string) (bool, bool, error) {
						return !glob.Contains(dirname), false, nil
					}
				} else if value.Type == eval.ScalarValueType {
					str := value.Value.(string)
					isDiscarderFnc = func(dirname string) (bool, bool, error) {
						return !strings.HasPrefix(str, dirname), false, nil
					}
				} else {
					// regex are not currently supported on path, see ValidateFields
					isDiscarderFnc = func(dirname string) (bool, bool, error) {
						return false, false, nil
					}
				}

				isDiscarderFncs = append(isDiscarderFncs, isDiscarderFnc)
			}
		}

		// collect all the rule on which we need to check the parent discarder found
		if values := rule.GetFieldValues(basenameField); len(values) > 0 {
			basenameRules = append(basenameRules, rule)
		}
	}

	// basename check, the goal is to ensure there is no dirname(parent) that matches a .file.name rule
	isDiscarderFnc = func(dirname string) (bool, bool, error) {
		if err := id.discarderEvent.SetFieldValue(basenameField, path.Base(dirname)); err != nil {
			return false, false, err
		}

		if isDiscarder, _ := rules.IsDiscarder(id.evalCtx, basenameField, basenameRules); !isDiscarder {
			return false, true, nil
		}

		return true, true, nil
	}
	isDiscarderFncs = append(isDiscarderFncs, isDiscarderFnc)

	fnc = func(dirname string) (bool, error) {
		var result, altered bool
		var err error

		defer func() {
			if altered {
				*id.discarderEvent = *eventZeroDiscarder
			}
		}()

		for _, fnc := range isDiscarderFncs {
			result, altered, err = fnc(dirname)
			if !result {
				return false, err
			}
		}

		return len(isDiscarderFncs) > 0, nil
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

func (id *inodeDiscarders) discardParentInode(req *erpc.ERPCRequest, rs *rules.RuleSet, eventType model.EventType, field eval.Field, filename string, mountID uint32, inode uint64, pathID uint32, timestamp uint64) (bool, uint32, uint64, error) {
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
		if err != nil || dentry.IsFakeInode(parentInode) {
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
type inodeEventGetter = func(event *model.Event) (eval.Field, *model.FileEvent, bool)

func filenameDiscarderWrapper(eventType model.EventType, getter inodeEventGetter) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *model.Event, probe *Probe, discarder Discarder) (bool, error) {
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
			if !isDiscarded && !isDeleted && err == nil {
				if !dentry.IsFakeInode(inode) {
					seclog.Tracef("Apply `%s.file.path` inode discarder for event `%s`, inode: %d(%s)", eventType, eventType, inode, filename)

					// not able to discard the parent then only discard the filename
					_ = probe.inodeDiscarders.discardInode(probe.erpcRequest, eventType, mountID, inode, true)
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

// PidDiscarderDump describes a dump of a pid discarder
type PidDiscarderDump struct {
	Index              int `yaml:"index"`
	PidDiscarderParams `yaml:"value"`
}

// InodeDiscarderDump describes a dump of an inode discarder
type InodeDiscarderDump struct {
	Index                int `yaml:"index"`
	InodeDiscarderParams `yaml:"value"`
	FilePath             string `yaml:"path"`
	Inode                uint64
	MountID              uint32 `yaml:"mount_id"`
}

// DiscardersDump describes a dump of discarders
type DiscardersDump struct {
	Date   time.Time                 `yaml:"date"`
	Inodes []InodeDiscarderDump      `yaml:"inodes"`
	Pids   []PidDiscarderDump        `yaml:"pids"`
	Stats  map[string]DiscarderStats `yaml:"stats"`
}

func dumpPidDiscarders(resolver *dentry.Resolver, pidMap *ebpf.Map) ([]PidDiscarderDump, error) {
	var dumps []PidDiscarderDump

	info, err := pidMap.Info()
	if err != nil {
		return nil, fmt.Errorf("could not get info about pid discarders: %w", err)
	}

	var (
		count     int
		pid       uint32
		pidParams PidDiscarderParams
	)

	for entries := pidMap.Iterate(); entries.Next(&pid, &pidParams); {
		record := PidDiscarderDump{
			Index:              count,
			PidDiscarderParams: pidParams,
		}

		dumps = append(dumps, record)

		count++
		if count == int(info.MaxEntries) {
			break
		}
	}

	return dumps, nil
}

func dumpInodeDiscarders(resolver *dentry.Resolver, inodeMap *ebpf.Map) ([]InodeDiscarderDump, error) {
	var dumps []InodeDiscarderDump

	info, err := inodeMap.Info()
	if err != nil {
		return nil, fmt.Errorf("could not get info about inode discarders: %w", err)
	}

	var (
		count       int
		inodeEntry  InodeDiscarderMapEntry
		inodeParams InodeDiscarderParams
	)

	for entries := inodeMap.Iterate(); entries.Next(&inodeEntry, &inodeParams); {
		record := InodeDiscarderDump{
			Index:                count,
			InodeDiscarderParams: inodeParams,
			Inode:                inodeEntry.PathKey.Inode,
			MountID:              inodeEntry.PathKey.MountID,
		}

		path, err := resolver.Resolve(inodeEntry.PathKey.MountID, inodeEntry.PathKey.Inode, inodeEntry.PathKey.PathID, false)
		if err == nil {
			record.FilePath = path
		}

		dumps = append(dumps, record)

		count++
		if count == int(info.MaxEntries) {
			break
		}
	}

	return dumps, nil
}

func dumpDiscarderStats(buffers ...*ebpf.Map) (map[string]DiscarderStats, error) {
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	stats := make(map[string]DiscarderStats)
	perCpu := make([]DiscarderStats, numCPU)

	var eventType uint32
	for _, buffer := range buffers {
		iterator := buffer.Iterate()

		for iterator.Next(&eventType, &perCpu) {
			for _, stat := range perCpu {
				key := model.EventType(eventType).String()

				entry, exists := stats[key]
				if !exists {
					stats[key] = DiscarderStats{
						DiscarderAdded: stat.DiscarderAdded,
						EventDiscarded: stat.EventDiscarded,
					}
				} else {
					entry.DiscarderAdded += stat.DiscarderAdded
					entry.EventDiscarded += stat.EventDiscarded
				}
			}
		}
	}

	return stats, nil
}

// DumpDiscarders removes all the discarders
func dumpDiscarders(resolver *dentry.Resolver, pidMap, inodeMap, statsFB, statsBB *ebpf.Map) (DiscardersDump, error) {
	seclog.Debugf("Dumping discarders")

	dump := DiscardersDump{
		Date: time.Now(),
	}

	pids, err := dumpPidDiscarders(resolver, pidMap)
	if err != nil {
		return dump, err
	}
	dump.Pids = pids

	inodes, err := dumpInodeDiscarders(resolver, inodeMap)
	if err != nil {
		return dump, err
	}
	dump.Inodes = inodes

	stats, err := dumpDiscarderStats(statsFB, statsBB)
	if err != nil {
		return dump, err
	}
	dump.Stats = stats

	return dump, nil
}

var invalidDiscarders map[eval.Field]map[interface{}]bool

func init() {
	invalidDiscarders = createInvalidDiscardersCache()

	allDiscarderHandlers["open"] = append(allDiscarderHandlers["open"], filenameDiscarderWrapper(model.FileOpenEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "open.file.path", &event.Open.File, false
		}))
	SupportedDiscarders["open.file.path"] = true

	allDiscarderHandlers["mkdir"] = append(allDiscarderHandlers["mkdir"], filenameDiscarderWrapper(model.FileMkdirEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "mkdir.file.path", &event.Mkdir.File, false
		}))
	SupportedDiscarders["mkdir.file.path"] = true

	allDiscarderHandlers["unlink"] = append(allDiscarderHandlers["unlink"], filenameDiscarderWrapper(model.FileUnlinkEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "unlink.file.path", &event.Unlink.File, true
		}))
	SupportedDiscarders["unlink.file.path"] = true

	allDiscarderHandlers["rmdir"] = append(allDiscarderHandlers["rmdir"], filenameDiscarderWrapper(model.FileRmdirEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "rmdir.file.path", &event.Rmdir.File, false
		}))
	SupportedDiscarders["rmdir.file.path"] = true

	allDiscarderHandlers["chmod"] = append(allDiscarderHandlers["chmod"], filenameDiscarderWrapper(model.FileChmodEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "chmod.file.path", &event.Chmod.File, false
		}))
	SupportedDiscarders["chmod.file.path"] = true

	allDiscarderHandlers["chown"] = append(allDiscarderHandlers["chown"], filenameDiscarderWrapper(model.FileChownEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "chown.file.path", &event.Chown.File, false
		}))
	SupportedDiscarders["chown.file.path"] = true

	allDiscarderHandlers["utimes"] = append(allDiscarderHandlers["utimes"], filenameDiscarderWrapper(model.FileUtimesEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "utimes.file.path", &event.Utimes.File, false
		}))
	SupportedDiscarders["utimes.file.path"] = true

	allDiscarderHandlers["setxattr"] = append(allDiscarderHandlers["setxattr"], filenameDiscarderWrapper(model.FileSetXAttrEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "setxattr.file.path", &event.SetXAttr.File, false
		}))
	SupportedDiscarders["setxattr.file.path"] = true

	allDiscarderHandlers["removexattr"] = append(allDiscarderHandlers["removexattr"], filenameDiscarderWrapper(model.FileRemoveXAttrEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "removexattr.file.path", &event.RemoveXAttr.File, false
		}))
	SupportedDiscarders["removexattr.file.path"] = true

	allDiscarderHandlers["mmap"] = append(allDiscarderHandlers["mmap"], filenameDiscarderWrapper(model.MMapEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "mmap.file.path", &event.MMap.File, false
		}))
	SupportedDiscarders["mmap.file.path"] = true

	allDiscarderHandlers["splice"] = append(allDiscarderHandlers["splice"], filenameDiscarderWrapper(model.SpliceEventType,
		func(event *model.Event) (eval.Field, *model.FileEvent, bool) {
			return "splice.file.path", &event.Splice.File, false
		}))
	SupportedDiscarders["splice.file.path"] = true
}
