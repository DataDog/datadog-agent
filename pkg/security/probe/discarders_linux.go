// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package probe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/discarder"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
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
	allEventTypes = math.MaxUint32 //nolint:unused

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

	allDiscarders = make(map[eval.Field]discarderDecl)
)

type discarderHandler func(rs *rules.RuleSet, event *model.Event, probe *EBPFProbe, discarder Discarder) (bool, error)

// discarderDecl declares a discarder, applied on the first of its fields
type discarderDecl struct {
	// fields are the fields whose values the discarder discards together
	fields  []eval.Field
	handler discarderHandler
}

func addDiscarder(handler discarderHandler, fields ...eval.Field) {
	allDiscarders[fields[0]] = discarderDecl{fields: fields, handler: handler}
}

// addFileDiscarder declares the inode discarder of an event type, which discards
// the path and the name of the file together
func addFileDiscarder(prefix string, eventType model.EventType, getter inodeEventGetter) {
	pathField := prefix + ".file.path"
	addDiscarder(filenameDiscarderWrapper(eventType, pathField, getter), pathField, prefix+".file.name")
}

// bumpDiscardersRevision sends an eRPC request to bump the discarders revisionr
func bumpDiscardersRevision(e *erpc.ERPC) error {
	req := erpc.NewERPCRequest(erpc.BumpDiscardersRevisionOp)
	return e.Request(req)
}

func marshalDiscardHeader(req *erpc.Request, eventType model.EventType, timeout uint64) int {
	binary.NativeEndian.PutUint64(req.Data[0:8], uint64(eventType))
	binary.NativeEndian.PutUint64(req.Data[8:16], timeout)

	return 16
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

// DiscarderParams describes a map value
type DiscarderParams struct {
	EventMask  uint64                                                                   `yaml:"event_mask"`
	Timestamps [model.LastDiscarderEventType + 1 - model.FirstDiscarderEventType]uint64 `yaml:"-"`
	ExpireAt   uint64                                                                   `yaml:"expire_at"`
	IsRetained uint32                                                                   `yaml:"is_retained"`
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

	// parentDiscarderFncs holds the parent discarder functions of rs
	parentDiscarderFncs map[eval.Field]func(dir, overlayDir string) bool

	recentlyAddedEntries [maxRecentlyAddedCacheSize]InodeDiscarderEntry
}

func newInodeDiscarders(erpc *erpc.ERPC, dentryResolver *dentry.Resolver) *inodeDiscarders {
	return &inodeDiscarders{
		erpc:           erpc,
		dentryResolver: dentryResolver,
	}
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

func (id *inodeDiscarders) discardInode(req *erpc.Request, eventType model.EventType, mountID uint32, inode uint64, isLeaf bool) error {
	var isLeafInt uint32
	if isLeaf {
		isLeafInt = 1
	}

	req.OP = erpc.DiscardInodeOp

	offset := marshalDiscardHeader(req, eventType, 0)
	binary.NativeEndian.PutUint64(req.Data[offset:offset+8], inode)
	binary.NativeEndian.PutUint32(req.Data[offset+8:offset+12], mountID)
	binary.NativeEndian.PutUint32(req.Data[offset+12:offset+16], isLeafInt)

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

// getParentDiscarderFnc returns whether a directory is a discarder of the files
// below it, no rule of the event type matching them, the field being their path.
// It is nil when no directory is. See literalBelow for overlayDir.
func getParentDiscarderFnc(rs *rules.RuleSet, eventType model.EventType, field eval.Field) (func(dir, overlayDir string) bool, error) {
	bucket := rs.GetBucket(eventType.String())
	if bucket == nil {
		return nil, nil
	}

	var canMatch []func(dir, overlayDir string) bool
	for _, rule := range bucket.GetRules() {
		below, err := canMatchBelow(rule.GetResolvedAst().Root, field, true)
		if below == nil || err != nil {
			return nil, err
		}
		canMatch = append(canMatch, below)
	}
	return func(dir, overlayDir string) bool {
		return !slices.ContainsFunc(canMatch, func(below func(dir, overlayDir string) bool) bool { return below(dir, overlayDir) })
	}, nil
}

// canMatchBelow returns whether the expression can be true below a directory,
// or false when it is under an odd number of negations. Only the comparisons of
// the field in a positive position are decided.
func canMatchBelow(node ast.Node, field eval.Field, positive bool) (func(dir, overlayDir string) bool, error) {
	switch n := node.(type) {
	case *ast.LogicalExpr:
		left, err := canMatchBelow(n.Left, field, positive)
		if err != nil {
			return nil, err
		}
		right, err := canMatchBelow(n.Right, field, positive)
		if err != nil {
			return nil, err
		}
		if (n.Op == ast.LogicalAnd) == positive {
			if left == nil {
				return right, nil
			}
			if right == nil {
				return left, nil
			}
			return func(dir, overlayDir string) bool { return left(dir, overlayDir) && right(dir, overlayDir) }, nil
		}
		if left == nil || right == nil {
			return nil, nil
		}
		return func(dir, overlayDir string) bool { return left(dir, overlayDir) || right(dir, overlayDir) }, nil
	case *ast.UnaryExpr:
		if n.Op == ast.UnaryNot {
			return canMatchBelow(n.Operand, field, !positive)
		}
	case *ast.ScalarCompareExpr:
		left, right := n.Left, n.Right
		if n.Op == ast.ScalarEqual && !isFieldRef(left, field) {
			left, right = right, left
		}
		literal, ok := right.(*ast.Literal)
		if ok && positive && isFieldRef(left, field) && (n.Op == ast.ScalarEqual || n.Op == ast.ScalarMatch) {
			return literalBelow(literal, n.Op == ast.ScalarMatch)
		}
	case *ast.ArrayCompareExpr:
		array, ok := n.Right.(*ast.Array)
		if !ok || !positive || !isFieldRef(n.Left, field) || n.Op != ast.ArrayIn {
			return nil, nil
		}
		members := make([]func(dir, overlayDir string) bool, 0, len(array.Members))
		for _, member := range array.Members {
			below, err := literalBelow(member, false)
			if below == nil || err != nil {
				return nil, err
			}
			members = append(members, below)
		}
		return func(dir, overlayDir string) bool {
			return slices.ContainsFunc(members, func(below func(dir, overlayDir string) bool) bool { return below(dir, overlayDir) })
		}, nil
	}
	return nil, nil
}

func isFieldRef(node ast.Node, field eval.Field) bool {
	ref, ok := node.(*ast.FieldRef)
	return ok && ref.Name == field
}

// literalBelow returns whether a path equal to the literal, or matching it, can
// be below a directory. As OverlayFSPathname also compares the path relative to
// its overlay mount point, so is the directory as overlayDir.
func literalBelow(literal *ast.Literal, match bool) (func(dir, overlayDir string) bool, error) {
	switch literal.Value.Kind() {
	case ast.String:
		if !match {
			path := ast.StringVal(literal.Value)
			return func(dir, overlayDir string) bool {
				return strings.HasPrefix(path, dir) || strings.HasPrefix(path, overlayDir)
			}, nil
		}
		fallthrough
	case ast.Pattern:
		glob, err := eval.NewGlob(ast.StringVal(literal.Value), false, false)
		if err != nil {
			return nil, err
		}
		return func(dir, overlayDir string) bool { return glob.IsPrefix(dir) || glob.IsPrefix(overlayDir) }, nil
	}
	return nil, nil
}

func (id *inodeDiscarders) isParentPathDiscarder(rs *rules.RuleSet, eventType model.EventType, field eval.Field, filename string, depth int, overlayMountPoint string) (bool, error) {
	if id.rs != rs {
		id.parentDiscarderFncs = make(map[eval.Field]func(dir, overlayDir string) bool)
		id.rs = rs
	}

	fnc, exists := id.parentDiscarderFncs[field]
	if !exists {
		var err error
		if fnc, err = getParentDiscarderFnc(rs, eventType, field); err != nil {
			return false, err
		}
		id.parentDiscarderFncs[field] = fnc
	}
	if fnc == nil {
		return false, nil
	}

	dirname := getParent(filename, depth)
	overlayDir := dirname
	if overlayMountPoint != "" {
		var below bool
		if overlayDir, below = strings.CutPrefix(dirname, overlayMountPoint); !below || !strings.HasPrefix(overlayDir, "/") {
			return false, nil
		}
	}
	if dirname == "/" || !fnc(dirname, overlayDir) {
		// never discard /
		return false, nil
	}

	seclog.Tracef("`%s` discovered as parent discarder for `%s`", dirname, field)

	return true, nil
}

func (id *inodeDiscarders) discardParentInode(req *erpc.Request, rs *rules.RuleSet, eventType model.EventType, field eval.Field, filename string, overlayMountPoint string, pathKey model.PathKey, timestamp uint64) (bool, uint32, uint64, error) {
	var discarderDepth int
	var isDiscarder bool
	var err error

	for depth := maxParentDiscarderDepth; depth > 0; depth-- {
		if isDiscarder, err = id.isParentPathDiscarder(rs, eventType, field, filename, depth, overlayMountPoint); isDiscarder {
			discarderDepth = depth
			break
		}
	}

	if err != nil || discarderDepth == 0 {
		return false, 0, 0, err
	}

	parentKey := pathKey

	for i := 0; i < discarderDepth; i++ {
		key, err := id.dentryResolver.GetParent(parentKey)
		if err != nil || model.IsFakeInode(pathKey.Inode) {
			if i == 0 {
				return false, 0, 0, err
			}
			break
		}
		parentKey = key
	}

	// do not insert multiple time the same discarder
	if id.isRecentlyAdded(parentKey.MountID, parentKey.Inode, timestamp) {
		return false, 0, 0, nil
	}

	if err := id.discardInode(req, eventType, parentKey.MountID, parentKey.Inode, false); err != nil {
		return false, 0, 0, err
	}

	id.recentlyAdded(parentKey.MountID, parentKey.Inode, timestamp)

	return true, parentKey.MountID, parentKey.Inode, nil
}

// function used to retrieve discarder information, FileEvent, file deleted
type inodeEventGetter = func(event *model.Event) (*model.FileEvent, bool)

func filenameDiscarderWrapper(eventType model.EventType, field eval.Field, getter inodeEventGetter) discarderHandler {
	return func(rs *rules.RuleSet, event *model.Event, probe *EBPFProbe, _ Discarder) (bool, error) {
		fileEvent, isDeleted := getter(event)

		if fileEvent.PathResolutionError != nil {
			return false, fileEvent.PathResolutionError
		}

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

		var overlayMountPoint string
		if event.FieldHandlers.ResolveFileFilesystem(event, fileEvent) == model.OverlayFS {
			overlayMountPoint = strings.TrimRight(fileEvent.MountPath, "/")
		}

		isDiscarded, _, parentInode, err := probe.inodeDiscarders.discardParentInode(probe.erpcRequest, rs, eventType, field, filename, overlayMountPoint, fileEvent.PathKey, event.TimestampRaw)
		if !isDiscarded && !isDeleted && err == nil {
			if !model.IsFakeInode(fileEvent.PathKey.Inode) {
				seclog.Tracef("Apply `%s.file.path` inode discarder for event `%s`, inode: %d(%s)", eventType, eventType, fileEvent.PathKey.Inode, filename)

				// not able to discard the parent then only discard the filename
				_ = probe.inodeDiscarders.discardInode(probe.erpcRequest, eventType, fileEvent.PathKey.MountID, fileEvent.PathKey.Inode, true)
			}
		} else if !isDeleted {
			seclog.Tracef("Apply `%s.file.path` parent inode discarder for event `%s`, inode: %d(%s)", eventType, eventType, parentInode, filename)
		}

		if err != nil {
			err = fmt.Errorf("unable to set inode discarders for `%s` for event `%s`, inode: %d: %w", filename, eventType, parentInode, err)
		}

		return true, err
	}
}

// isInvalidDiscarder returns whether the given value is a valid discarder for the given field
func isInvalidDiscarder(field eval.Field, value string) bool {
	return (strings.HasSuffix(field, ".file.path") || strings.HasSuffix(field, ".file.destination.path")) && value == ""
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
	Date   time.Time                  `yaml:"date"`
	Inodes []InodeDiscarderDump       `yaml:"inodes"`
	Stats  map[string]discarder.Stats `yaml:"stats"`
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

		path, err := resolver.Resolve(inodeEntry.PathKey, false, dentry.CallerDiscarder)
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

func dumpDiscarderStats(buffers ...*ebpf.Map) (map[string]discarder.Stats, error) {
	numCPU, err := ebpf.PossibleCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the possible CPU count: %w", err)
	}

	stats := make(map[string]discarder.Stats)
	perCPU := make([]discarder.Stats, numCPU)

	var eventType uint32
	for _, buffer := range buffers {
		iterator := buffer.Iterate()

		for iterator.Next(&eventType, &perCPU) {
			for _, stat := range perCPU {
				key := model.EventType(eventType).String()

				entry, exists := stats[key]
				if !exists {
					stats[key] = discarder.Stats{
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
func dumpDiscarders(resolver *dentry.Resolver, inodeMap, statsFB, statsBB *ebpf.Map) (DiscardersDump, error) {
	seclog.Debugf("Dumping discarders")

	dump := DiscardersDump{
		Date: time.Now(),
	}

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

func applyDNSDefaultDropMaskFromRules(manager *manager.Manager, rs *rules.RuleSet) error {
	bucket := rs.GetRuleBucket(model.DNSEventType.String())
	if bucket == nil {
		return setDNSDiscarderMask(manager, ^uint16(0))
	}

	var allowMask uint16

	for code := 0; code < 16; code++ {
		ev := rs.NewFakeEvent()
		_ = ev.SetFieldValue("dns.response.code", code)

		ctx := eval.NewContext(ev)

		isDiscarder := true
		for _, r := range bucket.GetRules() {
			needsFullResponse := ruleNeedsFullDNSResponse(r)

			ok, err := r.PartialEval(ctx, "dns.response.code")
			if err != nil {
				var nf *eval.ErrFieldNotFound
				if errors.As(err, &nf) {
					if needsFullResponse {
						isDiscarder = false
						break
					}
					continue
				}
				isDiscarder = false
				break
			}
			if ok {
				isDiscarder = false
				break
			}
		}

		if !isDiscarder {
			allowMask |= 1 << uint(code)
		}
	}

	return setDNSDiscarderMask(manager, ^allowMask)
}

func ruleNeedsFullDNSResponse(rule *rules.Rule) bool {
	for _, field := range rule.GetFields() {
		if strings.HasPrefix(field, "dns.response.") && field != "dns.response.code" {
			return true
		}
	}

	return false
}

func prCtlDiscarder(_ *rules.RuleSet, event *model.Event, probe *EBPFProbe, _ Discarder) (bool, error) {
	value, err := event.GetFieldValue("prctl.new_name")
	if err != nil {
		return false, err
	}

	name := value.(string)
	if len(name) > 16 {
		return false, errors.New("prctl name length exceeded the maximum of 16 bytes")
	}

	probe.erpcRequest.OP = erpc.DiscardPrctlOp
	probe.erpcRequest.Data = [256]byte{}
	copy(probe.erpcRequest.Data[:], name)
	err = probe.Erpc.Request(probe.erpcRequest)

	if err != nil {
		return false, err
	}

	return true, nil
}

func setDNSDiscarderMask(manager *manager.Manager, dnsMask uint16) error {
	bufferSelector, err := managerhelper.Map(manager, "filtered_dns_rcodes")
	if err != nil {
		return err
	}

	err = bufferSelector.Put(uint32(0), dnsMask)
	if err != nil {
		return err
	}

	seclog.Tracef("DNS discarder for response code: %d", dnsMask)
	return nil
}

func auidDiscarder(_ *rules.RuleSet, event *model.Event, probe *EBPFProbe, _ Discarder) (bool, error) {
	value, err := event.GetFieldValue("process.auid")
	if err != nil {
		return false, err
	}

	auid := value.(int)
	probe.erpcRequest.OP = erpc.DiscardAuidOp
	probe.erpcRequest.Data = [256]byte{}
	binary.LittleEndian.PutUint32(probe.erpcRequest.Data[:4], uint32(auid))
	binary.LittleEndian.PutUint64(probe.erpcRequest.Data[4:12], uint64(event.GetEventType()))
	err = probe.Erpc.Request(probe.erpcRequest)

	if err != nil {
		return false, err
	}

	return true, nil
}
func init() {
	addFileDiscarder("open", model.FileOpenEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.Open.File, false
	})
	addFileDiscarder("mkdir", model.FileMkdirEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.Mkdir.File, false
	})
	addFileDiscarder("unlink", model.FileUnlinkEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.Unlink.File, true
	})
	addFileDiscarder("rmdir", model.FileRmdirEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.Rmdir.File, false
	})
	addFileDiscarder("chmod", model.FileChmodEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.Chmod.File, false
	})
	addFileDiscarder("chown", model.FileChownEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.Chown.File, false
	})
	addFileDiscarder("utimes", model.FileUtimesEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.Utimes.File, false
	})
	addFileDiscarder("setxattr", model.FileSetXAttrEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.SetXAttr.File, false
	})
	addFileDiscarder("removexattr", model.FileRemoveXAttrEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.RemoveXAttr.File, false
	})
	addFileDiscarder("mmap", model.MMapEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.MMap.File, false
	})
	addFileDiscarder("splice", model.SpliceEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.Splice.File, false
	})
	addFileDiscarder("chdir", model.FileOpenEventType, func(event *model.Event) (*model.FileEvent, bool) {
		return &event.Open.File, false
	})

	addDiscarder(auidDiscarder, "process.auid")
	addDiscarder(prCtlDiscarder, "prctl.new_name")

	for field, d := range allDiscarders {
		SupportedDiscarders[field] = d.fields
	}
}
