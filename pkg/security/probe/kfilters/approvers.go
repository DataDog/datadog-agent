// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"path"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// BasenameApproverKernelMapName defines the basename approver kernel map name
const BasenameApproverKernelMapName = "basename_approvers"

type kfiltersGetter func(approvers rules.Approvers) (ActiveKFilters, error)

// KFilterGetters var contains all the kfilter getters
var KFilterGetters = make(map[eval.EventType]kfiltersGetter)

func newBasenameKFilter(tableName string, eventType model.EventType, basename string) (activeKFilter, error) {
	return &eventMaskEntry{
		tableName: tableName,
		tableKey:  ebpf.NewStringMapItem(basename, BasenameFilterSize),
		eventMask: uint64(1 << (eventType - 1)),
	}, nil
}

func newBasenameKFilters(tableName string, eventType model.EventType, basenames ...string) (approvers []activeKFilter, _ error) {
	for _, basename := range basenames {
		activeKFilter, err := newBasenameKFilter(tableName, eventType, basename)
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

func newKFilterWithUInt32Flags(tableName string, flags ...uint32) (activeKFilter, error) {
	var bitmask uint32
	for _, flag := range flags {
		bitmask |= flag
	}

	return &arrayEntry{
		tableName: tableName,
		index:     uint32(0),
		value:     ebpf.NewUint32FlagsMapItem(bitmask),
		zeroValue: ebpf.Uint32FlagsZeroMapItem,
	}, nil
}

func newKFilterWithUInt64Flags(tableName string, flags ...uint64) (activeKFilter, error) {
	var bitmask uint64
	for _, flag := range flags {
		bitmask |= flag
	}

	return &arrayEntry{
		tableName: tableName,
		index:     uint32(0),
		value:     ebpf.NewUint64FlagsMapItem(bitmask),
		zeroValue: ebpf.Uint64FlagsZeroMapItem,
	}, nil
}

func getFlagsKFilter(tableName string, flags ...uint32) (activeKFilter, error) {
	return newKFilterWithUInt32Flags(tableName, flags...)
}

func getEnumsKFilters(tableName string, enums ...uint64) (activeKFilter, error) {
	var flags []uint64
	for _, enum := range enums {
		flags = append(flags, 1<<enum)
	}
	return newKFilterWithUInt64Flags(tableName, flags...)
}

func getBasenameKFilters(eventType model.EventType, field string, approvers rules.Approvers) ([]activeKFilter, error) {
	stringValues := func(fvs rules.FilterValues) []string {
		var values []string
		for _, v := range fvs {
			values = append(values, v.Value.(string))
		}
		return values
	}

	prefix := eventType.String()
	if field != "" {
		prefix += "." + field
	}

	var kfilters []activeKFilter
	for field, values := range approvers {
		switch field {
		case prefix + model.NameSuffix:
			activeKFilters, err := newBasenameKFilters(BasenameApproverKernelMapName, eventType, stringValues(values)...)
			if err != nil {
				return nil, err
			}
			kfilters = append(kfilters, activeKFilters...)

		case prefix + model.PathSuffix:
			for _, value := range stringValues(values) {
				basename := path.Base(value)
				activeKFilter, err := newBasenameKFilter(BasenameApproverKernelMapName, eventType, basename)
				if err != nil {
					return nil, err
				}
				kfilters = append(kfilters, activeKFilter)
			}
		}
	}

	return kfilters, nil
}

func fimKFiltersGetter(event model.EventType, fields []eval.Field) kfiltersGetter {
	return func(approvers rules.Approvers) (ActiveKFilters, error) {
		var kfilters []activeKFilter
		for _, field := range fields {
			kfilter, err := getBasenameKFilters(event, field, approvers)
			if err != nil {
				return nil, err
			}
			kfilters = append(kfilters, kfilter...)
		}
		return newActiveKFilters(kfilters...), nil
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
}
