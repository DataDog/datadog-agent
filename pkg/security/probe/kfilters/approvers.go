// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"fmt"
	"path"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/sharedconsts"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	// BasenameApproverKernelMapName defines the basename approver kernel map name
	BasenameApproverKernelMapName = "basename_approvers"
	// InUpperLayerApproverKernelMapName defines the in upper layer approver kernel map name
	InUpperLayerApproverKernelMapName = "in_upper_layer_approvers"

	// PolicyApproverType is the type of policy approver
	PolicyApproverType = "policy"
	// BasenameApproverType is the type of basename approver
	BasenameApproverType = "basename"
	// FlagApproverType is the type of flags approver
	FlagApproverType = "flag"
	// AUIDApproverType is the type of auid approver
	AUIDApproverType = "auid"
	// InUpperLayerApproverType is the type of in upper layer approver
	InUpperLayerApproverType = "in_upper_layer"

	patternAffixSize = 3 // size of the prefix and suffix basename keys, has to be in sync with the kernel side of the approvers
	// minParentBasenameSize avoids registering a parent-basename approver on a very short directory name
	// (e.g. "a"), which would match files in any same-named directory across the filesystem and make the
	// kernel approver too coarse to filter anything useful.
	minParentBasenameSize = 3

	auidApproversTable      = "auid_approvers"
	auidRangeApproversTable = "auid_range_approvers"
)

// Basename approver types — kept in sync with enum BASENAME_APPROVER_TYPE on the kernel side.
const (
	// leafBasename matches the event's own basename exactly.
	leafBasename uint8 = iota
	// leafBasenamePrefix matches the first patternAffixSize bytes of the event's basename
	// (used for wildcard rules whose leaf has a usable fixed prefix).
	leafBasenamePrefix
	// parentBasename matches the basename of the event's parent directory exactly
	// (used for wildcard rules whose leaf has no usable prefix).
	parentBasename
	// leafBasenameSuffix matches the last patternAffixSize bytes of the event's basename
	// (used for wildcard rules whose leaf is `*<suffix>`, like `*.php`).
	leafBasenameSuffix
)

// basenameApprover is the userspace form of struct basename_t: a one-byte type tag followed by
// a fixed-length, NUL-padded value. MarshalBinary produces a byte-for-byte match with the kernel
// struct so it can be used as a map key.
type basenameApprover struct {
	kind  uint8
	value string
}

// MarshalBinary serialises basenameApprover as `[type][value padded to BasenameFilterSize]`.
func (ba *basenameApprover) MarshalBinary() ([]byte, error) {
	b := make([]byte, 1+BasenameFilterSize)
	b[0] = ba.kind

	n := BasenameFilterSize - 1 // leave room for trailing \0
	if len(ba.value) < n {
		n = len(ba.value)
	}
	copy(b[1:], ba.value[:n])

	return b, nil
}

// The kernel approvers, one type per test of the kernel.

// basename approves the files on their basename, or on the basename of their
// parent directory.
type basename struct {
	key basenameApprover
}

// a parent-basename approver approves the files of every directory of that
// name, it is weighed below the flags approvers
func (a basename) Weight() int {
	if a.key.kind == parentBasename {
		return 20
	}
	return 300
}

// bits approves the events whose flags have one of some bits set, the bits of a
// table being ORed. No bits approves the events with no flags set.
type bits struct {
	table  string
	bits   int
	weight int
}

func (a bits) Weight() int { return a.weight }

// rdonly approves the events opening a file read-only.
type rdonly struct{}

func (rdonly) Weight() int { return 100 }

// enum approves the events whose field has a value, the values of a table index
// being ORed in a bitmask.
type enum struct {
	table  string
	index  uint32
	value  int
	weight int
}

func (a enum) Weight() int { return a.weight }

// inetFamily approves the events of an INET socket, the family of a table
// being AF_INET or AF_INET6.
type inetFamily struct {
	table string
}

// less selective than one family
func (inetFamily) Weight() int { return 40 }

// auid approves the events of the processes of an audit uid.
type auid struct {
	value int
}

func (auid) Weight() int { return 100 }

func (a auid) Report() (eval.FieldValueType, interface{}) {
	return eval.ScalarValueType, a.value
}

// Intersect intersects the audit uid with another one or with a range.
func (a auid) Intersect(other rules.Approver) ([]rules.Approver, bool) {
	switch o := other.(type) {
	case auid:
		if a == o {
			return approvers(a)
		}
	case auidRange:
		if o.min <= a.value && a.value <= o.max {
			return approvers(a)
		}
	default:
		return nil, false
	}
	return nil, true
}

// auidRange approves the events of the processes of a range of audit uids, the
// ranges of an event type being merged in the range covering them.
type auidRange struct {
	min, max int
}

func (auidRange) Weight() int { return 100 }

func (a auidRange) Report() (eval.FieldValueType, interface{}) {
	return eval.RangeValueType, rules.RangeFilterValue{Min: a.min, Max: a.max}
}

// Intersect intersects the range with another one or with an audit uid.
func (a auidRange) Intersect(other rules.Approver) ([]rules.Approver, bool) {
	switch o := other.(type) {
	case auid:
		return o.Intersect(a)
	case auidRange:
		r := auidRange{min: max(a.min, o.min), max: min(a.max, o.max)}
		if r.min > r.max {
			return nil, true
		}
		return approvers(r)
	}
	return nil, false
}

// upperLayer approves the events on the files of the upper layer of an overlay
// filesystem.
type upperLayer struct{}

func (upperLayer) Weight() int { return 50 }

// the events approved on their audit uid bypass the discarders

func (basename) ApproverOnly() bool   { return false }
func (bits) ApproverOnly() bool       { return false }
func (rdonly) ApproverOnly() bool     { return false }
func (enum) ApproverOnly() bool       { return false }
func (inetFamily) ApproverOnly() bool { return false }
func (auid) ApproverOnly() bool       { return true }
func (auidRange) ApproverOnly() bool  { return true }
func (upperLayer) ApproverOnly() bool { return false }

// The approver models, a comparison having the approvers of the kernel tests it
// implies.

func approvers(approvers ...rules.Approver) ([]rules.Approver, bool) {
	return approvers, true
}

// isSuffixPattern reports whether the literal text after the last '*' of pattern, which every
// match ends with, is long enough for the kernel suffix approver (`*.php`, `ab*.php`, `*foo*.php`).
func isSuffixPattern(pattern string) bool {
	i := strings.LastIndex(pattern, "*")
	return i >= 0 && len(pattern)-i-1 >= patternAffixSize
}

// Basename approves the paths of the files, or their names, on their basename.
// A pattern is approved on the prefix of its basename, else on its suffix, else,
// with parent, on the basename of its parent directory.
func Basename(parent bool) rules.ApproverModel {
	return rules.ApproverModel(func(c rules.Comparison) ([]rules.Approver, bool) {
		if value, ok := c.Equal(); ok {
			return approvers(basename{key: basenameApprover{kind: leafBasename, value: path.Base(value)}})
		}

		pattern, ok := c.Glob()
		if !ok {
			return nil, false
		}

		base := path.Base(pattern)
		if prefix, _, isPattern := strings.Cut(base, "*"); !isPattern {
			return approvers(basename{key: basenameApprover{kind: leafBasename, value: base}})
		} else if len(prefix) >= patternAffixSize {
			return approvers(basename{key: basenameApprover{kind: leafBasenamePrefix, value: prefix[:patternAffixSize]}})
		} else if isSuffixPattern(base) {
			return approvers(basename{key: basenameApprover{kind: leafBasenameSuffix, value: base[len(base)-patternAffixSize:]}})
		}

		// `**` matches the files of the subdirectories too
		dir := path.Base(path.Dir(pattern))
		if !parent || base == "**" || len(dir) < minParentBasenameSize || strings.Contains(dir, "*") {
			return nil, false
		}
		return approvers(basename{key: basenameApprover{kind: parentBasename, value: dir}})
	})
}

// Bits approves the flags having one of some bits set. With zero, it approves
// the flags having none set, `field == 0`.
func Bits(table string, weight int, zero bool) rules.ApproverModel {
	return rules.ApproverModel(func(c rules.Comparison) ([]rules.Approver, bool) {
		if value, ok := c.AnyBits(); ok {
			return approvers(bits{table: table, bits: value, weight: weight})
		}
		if mask, ok := c.NoBits(); ok && zero && mask == -1 {
			return approvers(bits{table: table, weight: weight})
		}
		return nil, false
	})
}

// RdOnly approves the flags of a read-only open, none of the bits of the access
// mode set.
var RdOnly = rules.ApproverModel(func(c rules.Comparison) ([]rules.Approver, bool) {
	if mask, ok := c.NoBits(); ok && mask&unix.O_ACCMODE == unix.O_ACCMODE {
		return approvers(rdonly{})
	}
	return nil, false
})

// Enum approves an enumeration field on its value.
func Enum(table string, index uint32, weight int) rules.ApproverModel {
	return rules.ApproverModel(func(c rules.Comparison) ([]rules.Approver, bool) {
		// the fields are unsigned, a negative value is none of them
		if value, ok := c.Int(); ok && value >= 0 {
			return approvers(enum{table: table, index: index, value: value, weight: weight})
		}
		return nil, false
	})
}

// InetFamily approves the fields of an address on its family, the address being
// of an INET socket for the values an address of another family doesn't have:
// a port, an IP that isn't public, an IP or a hostname.
func InetFamily(table string) rules.ApproverModel {
	return rules.ApproverModel(func(c rules.Comparison) ([]rules.Approver, bool) {
		port, isPort := c.Int()
		isPublic, isBool := c.Bool()
		hostname, isHostname := c.Equal()
		_, isIP := c.IP()
		if isPort && port != 0 || isBool && !isPublic || isHostname && hostname != "" || isIP {
			return approvers(inetFamily{table: table})
		}
		return nil, false
	})
}

// AUID approves the audit uid, a value or a range of values, the unset audit uid
// being the highest one.
var AUID = rules.ApproverModel(func(c rules.Comparison) ([]rules.Approver, bool) {
	if value, ok := c.Int(); ok {
		return approvers(auid{value: value})
	}
	if value, ok := c.NotInt(); ok && value == sharedconsts.AuditUIDUnset {
		return approvers(auidRange{min: 0, max: sharedconsts.AuditUIDUnset - 1})
	}
	if minimum, maximum, ok := c.Range(); ok {
		return approvers(auidRange{min: max(minimum, 0), max: min(maximum, sharedconsts.AuditUIDUnset)})
	}
	return nil, false
})

// UpperLayer approves the files of the upper layer of an overlay filesystem.
var UpperLayer = rules.ApproverModel(func(c rules.Comparison) ([]rules.Approver, bool) {
	if inUpperLayer, ok := c.Bool(); ok && inUpperLayer {
		return approvers(upperLayer{})
	}
	return nil, false
})

// NewKFilters returns the kernel filters of the approvers of an event type.
func NewKFilters(eventType eval.EventType, approvers rules.Approvers) (KFilters, error) {
	event, err := model.ParseEvalEventType(eventType)
	if err != nil {
		return nil, err
	}
	eventMask := uint64(1 << (event - 1))

	type enumKey struct {
		table string
		index uint32
	}
	var (
		kfilters    = newKFilters()
		flags       = make(map[string]int)
		noFlags     = make(map[string]bool)
		enums       = make(map[enumKey]uint64)
		auids       *rules.RangeFilterValue
		inetFamilys = []int{unix.AF_INET, unix.AF_INET6}
	)

	for _, values := range approvers {
		for _, value := range values {
			switch a := value.Approver.(type) {
			case basename:
				kfilters.Add(&eventMaskKFilter{approverType: BasenameApproverType, tableName: BasenameApproverKernelMapName, tableKey: &a.key, eventMask: eventMask})
			case bits:
				flags[a.table] |= a.bits
				noFlags[a.table] = noFlags[a.table] || a.bits == 0
			case rdonly:
				kfilters.Add(&arrayKFilter{approverType: FlagApproverType, tableName: "open_flags_rdonly_approver", index: uint32(0), value: ebpf.BoolTrueMapItem, zeroValue: ebpf.BoolFalseMapItem})
			case enum:
				enums[enumKey{a.table, a.index}] |= 1 << (a.value % 64)
			case inetFamily:
				for _, family := range inetFamilys {
					enums[enumKey{a.table, 0}] |= 1 << (family % 64)
				}
			case auid:
				kfilters.Add(&eventMaskKFilter{approverType: AUIDApproverType, tableName: auidApproversTable, tableKey: ebpf.Uint32MapItem(a.value), eventMask: eventMask})
			case auidRange:
				if auids == nil {
					auids = &rules.RangeFilterValue{Min: a.min, Max: a.max}
				}
				auids.Min, auids.Max = min(auids.Min, a.min), max(auids.Max, a.max)
			case upperLayer:
				kfilters.Add(&eventMaskKFilter{approverType: InUpperLayerApproverType, tableName: InUpperLayerApproverKernelMapName, tableKey: ebpf.Uint32MapItem(0), eventMask: eventMask, isArrayMap: true})
			default:
				return nil, fmt.Errorf("unknown approver %T of %s", value.Approver, value.Field)
			}
		}
	}

	for table, bits := range flags {
		// the kernel approves no flags only when no bits are approved
		if noFlags[table] && bits != 0 {
			return nil, fmt.Errorf("no flags and some flags can't be both approved by %s", table)
		}
		kfilters.Add(&arrayKFilter{approverType: FlagApproverType, tableName: table, index: uint32(0), value: ebpf.NewUint32FlagsMapItem(uint32(bits)), zeroValue: ebpf.Uint32FlagsZeroMapItem})
	}
	for key, bitmask := range enums {
		kfilters.Add(&arrayKFilter{approverType: FlagApproverType, tableName: key.table, index: key.index, value: ebpf.NewUint64FlagsMapItem(bitmask), zeroValue: ebpf.Uint64FlagsZeroMapItem})
	}
	if auids != nil {
		kfilters.Add(&hashKFilter{approverType: AUIDApproverType, tableName: auidRangeApproversTable, tableKey: event, value: ebpf.NewUInt32RangeMapItem(uint32(auids.Min), uint32(auids.Max))})
	}

	return kfilters, nil
}
