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
	return &mapEventMask{
		tableName: tableName,
		key:       basename,
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

func intValues[I int32 | int64](fvs rules.FilterValues) []I {
	var values []I
	for _, v := range fvs {
		values = append(values, I(v.Value.(int)))
	}
	return values
}

func newKFilterWithFlags[I int32 | int64](tableName string, flags ...I) (activeKFilter, error) {
	var flagsItem I

	for _, flag := range flags {
		flagsItem |= flag
	}

	if flagsItem != 0 {
		return &arrayEntry{
			tableName: tableName,
			index:     uint32(0),
			value:     flagsItem,
			zeroValue: I(0),
		}, nil
	}

	return nil, nil
}

func getFlagsKFilters(tableName string, flags ...int32) (activeKFilter, error) {
	return newKFilterWithFlags(tableName, flags...)
}

func getEnumsKFilters(tableName string, enums ...int64) (activeKFilter, error) {
	var flags []int64
	for _, enum := range enums {
		flags = append(flags, 1<<enum)
	}
	return newKFilterWithFlags(tableName, flags...)
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

func basenameKFilterGetter(event model.EventType) kfiltersGetter {
	return func(approvers rules.Approvers) (ActiveKFilters, error) {
		basenameKFilters, err := getBasenameKFilters(event, "file", approvers)
		if err != nil {
			return nil, err
		}
		return newActiveKFilters(basenameKFilters...), nil
	}
}

func basenameskfiltersGetter(event model.EventType, field1, field2 string) kfiltersGetter {
	return func(approvers rules.Approvers) (ActiveKFilters, error) {
		basenameKFilters, err := getBasenameKFilters(event, field1, approvers)
		if err != nil {
			return nil, err
		}
		basenameKFilters2, err := getBasenameKFilters(event, field2, approvers)
		if err != nil {
			return nil, err
		}
		basenameKFilters = append(basenameKFilters, basenameKFilters2...)
		return newActiveKFilters(basenameKFilters...), nil
	}
}

func init() {
	KFilterGetters["chmod"] = basenameKFilterGetter(model.FileChmodEventType)
	KFilterGetters["chown"] = basenameKFilterGetter(model.FileChownEventType)
	KFilterGetters["link"] = basenameskfiltersGetter(model.FileLinkEventType, "file", "file.destination")
	KFilterGetters["mkdir"] = basenameKFilterGetter(model.FileMkdirEventType)
	KFilterGetters["open"] = openOnNewApprovers
	KFilterGetters["rename"] = basenameskfiltersGetter(model.FileRenameEventType, "file", "file.destination")
	KFilterGetters["rmdir"] = basenameKFilterGetter(model.FileRmdirEventType)
	KFilterGetters["unlink"] = basenameKFilterGetter(model.FileUnlinkEventType)
	KFilterGetters["utimes"] = basenameKFilterGetter(model.FileUtimesEventType)
	KFilterGetters["mmap"] = mmapKFilters
	KFilterGetters["mprotect"] = mprotectKFilters
	KFilterGetters["splice"] = spliceKFilters
	KFilterGetters["chdir"] = basenameKFilterGetter(model.FileChdirEventType)
	KFilterGetters["bpf"] = bpfKFilters
}
