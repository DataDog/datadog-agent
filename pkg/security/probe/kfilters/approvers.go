// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"errors"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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
)

// Basename approver types — kept in sync with enum BASENAME_APPROVER_TYPE on the kernel side.
const (
	// leafBasename matches the event's own basename exactly.
	leafBasename uint8 = iota
	// leafBasenamePrefix matches the first patternPrefixSize bytes of the event's basename
	// (used for wildcard rules whose leaf has a usable fixed prefix).
	leafBasenamePrefix
	// parentBasename matches the basename of the event's parent directory exactly
	// (used for wildcard rules whose leaf has no usable prefix).
	parentBasename
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

type kfiltersGetter func(approvers rules.Approvers) (KFilters, []eval.Field, error)

// KFilterGetters var contains all the kfilter getters
var KFilterGetters = make(map[eval.EventType]kfiltersGetter)

func newBasenameKFilter(tableName string, eventType model.EventType, basename string, valueType eval.FieldValueType) (kFilter, error) {
	basenameType := leafBasename

	if valueType == eval.PatternValueType || valueType == eval.GlobValueType {
		if strings.Contains(basename, "*") {
			// Reduce to a fixed-length prefix so the kernel can match it with a single
			// map lookup using the same shape built from the event basename.
			// validateScalarPathFilter guarantees len(els[0]) >= patternPrefixSize, so
			// the slice below is safe.
			els := strings.Split(basename, "*")
			if len(els[0]) < patternPrefixSize {
				return nil, errors.New("unexpected pattern prefix size")
			}
			basenameType = leafBasenamePrefix
			basename = els[0][:patternPrefixSize]
		}
	}

	return &eventMaskKFilter{
		approverType: BasenameApproverType,
		tableName:    tableName,
		tableKey:     &basenameApprover{kind: basenameType, value: basename},
		eventMask:    uint64(1 << (eventType - 1)),
	}, nil
}

func newParentBasenameKFilter(tableName string, eventType model.EventType, basename string) (kFilter, error) {
	return &eventMaskKFilter{
		approverType: BasenameApproverType,
		tableName:    tableName,
		tableKey:     &basenameApprover{kind: parentBasename, value: basename},
		eventMask:    uint64(1 << (eventType - 1)),
	}, nil
}

func newInUpperLayerKFilter(tableName string, eventType model.EventType) (kFilter, error) {
	return &eventMaskKFilter{
		approverType: InUpperLayerApproverType,
		tableName:    tableName,
		tableKey:     ebpf.Uint32MapItem(0),
		eventMask:    uint64(1 << (eventType - 1)),
		isArrayMap:   true,
	}, nil
}

func newBasenameKFilters(tableName string, eventType model.EventType, fvs ...rules.FilterValue) (approvers []kFilter, _ error) {
	for _, fv := range fvs {
		value, ok := fv.Value.(string)
		if !ok {
			return nil, errors.New("wrong basename value type")
		}
		basename := path.Base(value)

		activeKFilter, err := newBasenameKFilter(tableName, eventType, basename, fv.Type)
		if err != nil {
			return nil, err
		}
		approvers = append(approvers, activeKFilter)
	}
	return approvers, nil
}

func newPathKFilters(tableName string, eventType model.EventType, fvs ...rules.FilterValue) (approvers []kFilter, _ error) {
	for _, fv := range fvs {
		value, ok := fv.Value.(string)
		if !ok {
			return nil, errors.New("wrong path value type")
		}
		basename := path.Base(value)

		if fv.Type == eval.PatternValueType || fv.Type == eval.GlobValueType {
			if strings.Contains(basename, "*") {
				els := strings.Split(basename, "*")

				// When the leaf's pre-'*' prefix is shorter than patternPrefixSize, the kernel
				// prefix-match approver is too coarse to be useful (or impossible to build when
				// the basename is just "*"). Fall back to approving on the parent directory
				// basename instead, which the resolver looks up exactly.
				if len(els[0]) < patternPrefixSize {
					basename = path.Base(path.Dir(value))
					// should have been validated by the capabilities but worth validating here again
					if strings.Contains(basename, "*") {
						return nil, errors.New("wildcard on parent basename is not supported")
					}

					activeKFilter, err := newParentBasenameKFilter(tableName, eventType, basename)
					if err != nil {
						return nil, err
					}
					approvers = append(approvers, activeKFilter)

					continue
				}
			}
		}

		activeKFilter, err := newBasenameKFilter(tableName, eventType, basename, fv.Type)
		if err != nil {
			return nil, err
		}
		approvers = append(approvers, activeKFilter)
	}
	return approvers, nil
}

func uintValues[I uint32 | uint64](fvs rules.FilterValues) []I {
	var values []I
	for _, v := range fvs {
		values = append(values, I(v.Value.(int)))
	}
	return values
}

func newKFilterWithUInt32Flags(tableName string, flags ...uint32) (kFilter, error) {
	var bitmask uint32
	for _, flag := range flags {
		bitmask |= flag
	}

	return &arrayKFilter{
		approverType: FlagApproverType,
		tableName:    tableName,
		index:        uint32(0),
		value:        ebpf.NewUint32FlagsMapItem(bitmask),
		zeroValue:    ebpf.Uint32FlagsZeroMapItem,
	}, nil
}

func newKFilterWithUInt64FlagsAndIndex(tableName string, index uint32, flags ...uint64) (kFilter, error) {
	var bitmask uint64
	for _, flag := range flags {
		bitmask |= flag
	}

	return &arrayKFilter{
		approverType: FlagApproverType,
		tableName:    tableName,
		index:        index,
		value:        ebpf.NewUint64FlagsMapItem(bitmask),
		zeroValue:    ebpf.Uint64FlagsZeroMapItem,
	}, nil
}

func newKFilterZeroFlagValue(tableName string, approve bool) (kFilter, error) {
	mapValue := ebpf.BoolFalseMapItem
	if approve {
		mapValue = ebpf.BoolTrueMapItem
	}

	return &arrayKFilter{
		approverType: FlagApproverType,
		tableName:    tableName,
		index:        uint32(0),
		value:        mapValue,
		zeroValue:    ebpf.BoolFalseMapItem,
	}, nil
}

func getFlagsKFilter(tableName string, flags ...uint32) (kFilter, error) {
	return newKFilterWithUInt32Flags(tableName, flags...)
}

func getEnumsKFiltersWithIndex(tableName string, index uint32, enums ...uint64) (kFilter, error) {
	var flags []uint64
	for _, enum := range enums {
		flags = append(flags, 1<<(enum%64))
	}
	return newKFilterWithUInt64FlagsAndIndex(tableName, index, flags...)
}

func getBasenameKFilters(eventType model.EventType, field string, approvers rules.Approvers) ([]kFilter, []eval.Field, error) {
	var fieldHandled []eval.Field

	prefix := eventType.String()
	if field != "" {
		prefix += "." + field
	}

	var kfilters []kFilter
	for field, values := range approvers {
		switch field {
		case prefix + model.NameSuffix:
			activeKFilters, err := newBasenameKFilters(BasenameApproverKernelMapName, eventType, values...)
			if err != nil {
				return nil, nil, err
			}
			kfilters = append(kfilters, activeKFilters...)
			fieldHandled = append(fieldHandled, field)
		case prefix + model.PathSuffix:
			activeKFilters, err := newPathKFilters(BasenameApproverKernelMapName, eventType, values...)
			if err != nil {
				return nil, nil, err
			}
			kfilters = append(kfilters, activeKFilters...)
			fieldHandled = append(fieldHandled, field)
		}
	}

	return kfilters, fieldHandled, nil
}

func fimKFiltersGetter(eventType model.EventType, fields []eval.Field) kfiltersGetter {
	return func(approvers rules.Approvers) (KFilters, []eval.Field, error) {
		var (
			kfilters     []kFilter
			fieldHandled []eval.Field
		)

		for _, field := range fields {
			kfilter, handled, err := getBasenameKFilters(eventType, field, approvers)
			if err != nil {
				return nil, nil, err
			}
			kfilters = append(kfilters, kfilter...)
			fieldHandled = append(fieldHandled, handled...)
		}

		kfs, handled, err := getProcessKFilters(eventType, approvers)
		if err != nil {
			return nil, nil, err
		}
		kfilters = append(kfilters, kfs...)
		fieldHandled = append(fieldHandled, handled...)

		return newKFilters(kfilters...), fieldHandled, nil
	}
}

func init() {
	KFilterGetters["chmod"] = fimKFiltersGetter(model.FileChmodEventType, []eval.Field{"file"})
	KFilterGetters["chown"] = fimKFiltersGetter(model.FileChownEventType, []eval.Field{"file"})
	KFilterGetters["link"] = fimKFiltersGetter(model.FileLinkEventType, []eval.Field{"file", "file.destination"})
	KFilterGetters["mkdir"] = fimKFiltersGetter(model.FileMkdirEventType, []eval.Field{"file"})
	KFilterGetters["open"] = openKFiltersGetter
	KFilterGetters["rename"] = fimKFiltersGetter(model.FileRenameEventType, []eval.Field{"file", "file.destination"})
	KFilterGetters["rmdir"] = fimKFiltersGetter(model.FileRmdirEventType, []eval.Field{"file"})
	KFilterGetters["unlink"] = fimKFiltersGetter(model.FileUnlinkEventType, []eval.Field{"file"})
	KFilterGetters["utimes"] = fimKFiltersGetter(model.FileUtimesEventType, []eval.Field{"file"})
	KFilterGetters["mmap"] = mmapKFiltersGetter
	KFilterGetters["mprotect"] = mprotectKFiltersGetter
	KFilterGetters["splice"] = spliceKFiltersGetter
	KFilterGetters["chdir"] = fimKFiltersGetter(model.FileChdirEventType, []eval.Field{"file"})
	KFilterGetters["bpf"] = bpfKFiltersGetter
	KFilterGetters["sysctl"] = sysctlKFiltersGetter
	KFilterGetters["connect"] = connectKFiltersGetter
	KFilterGetters["prctl"] = prctlKFiltersGetter
	KFilterGetters["setsockopt"] = setsockoptKFiltersGetter
	KFilterGetters["socket"] = socketKFiltersGetter
}
