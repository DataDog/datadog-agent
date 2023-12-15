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

type onApproverHandler func(approvers rules.Approvers) (ActiveApprovers, error)
type activeApprover = activeKFilter

// ActiveApprovers defines the active approvers type
type ActiveApprovers = activeKFilters

// AllApproversHandlers var contains all the approvers handlers
var AllApproversHandlers = make(map[eval.EventType]onApproverHandler)

func approveBasename(tableName string, eventType model.EventType, basename string) (activeApprover, error) {
	return &mapEventMask{
		tableName: tableName,
		key:       basename,
		tableKey:  ebpf.NewStringMapItem(basename, BasenameFilterSize),
		eventMask: uint64(1 << (eventType - 1)),
	}, nil
}

func approveBasenames(tableName string, eventType model.EventType, basenames ...string) (approvers []activeApprover, _ error) {
	for _, basename := range basenames {
		activeApprover, err := approveBasename(tableName, eventType, basename)
		if err != nil {
			return nil, err
		}
		approvers = append(approvers, activeApprover)
	}
	return approvers, nil
}

func setFlagsFilter(tableName string, flags ...int) (activeApprover, error) {
	var flagsItem ebpf.Uint32MapItem

	for _, flag := range flags {
		flagsItem |= ebpf.Uint32MapItem(flag)
	}

	if flagsItem != 0 {
		return &arrayEntry{
			tableName: tableName,
			index:     uint32(0),
			value:     flagsItem,
			zeroValue: ebpf.ZeroUint32MapItem,
		}, nil
	}

	return nil, nil
}

func approveFlags(tableName string, flags ...int) (activeApprover, error) {
	return setFlagsFilter(tableName, flags...)
}

func onNewBasenameApprovers(eventType model.EventType, field string, approvers rules.Approvers) ([]activeApprover, error) {
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

	var basenameApprovers []activeApprover
	for field, values := range approvers {
		switch field {
		case prefix + model.NameSuffix:
			activeApprovers, err := approveBasenames(BasenameApproverKernelMapName, eventType, stringValues(values)...)
			if err != nil {
				return nil, err
			}
			basenameApprovers = append(basenameApprovers, activeApprovers...)

		case prefix + model.PathSuffix:
			for _, value := range stringValues(values) {
				basename := path.Base(value)
				activeApprover, err := approveBasename(BasenameApproverKernelMapName, eventType, basename)
				if err != nil {
					return nil, err
				}
				basenameApprovers = append(basenameApprovers, activeApprover)
			}
		}
	}

	return basenameApprovers, nil
}

func onNewBasenameApproversWrapper(event model.EventType) onApproverHandler {
	return func(approvers rules.Approvers) (ActiveApprovers, error) {
		basenameApprovers, err := onNewBasenameApprovers(event, "file", approvers)
		if err != nil {
			return nil, err
		}
		return newActiveKFilters(basenameApprovers...), nil
	}
}

func onNewTwoBasenamesApproversWrapper(event model.EventType, field1, field2 string) onApproverHandler {
	return func(approvers rules.Approvers) (ActiveApprovers, error) {
		basenameApprovers, err := onNewBasenameApprovers(event, field1, approvers)
		if err != nil {
			return nil, err
		}
		basenameApprovers2, err := onNewBasenameApprovers(event, field2, approvers)
		if err != nil {
			return nil, err
		}
		basenameApprovers = append(basenameApprovers, basenameApprovers2...)
		return newActiveKFilters(basenameApprovers...), nil
	}
}

func init() {
	AllApproversHandlers["chmod"] = onNewBasenameApproversWrapper(model.FileChmodEventType)
	AllApproversHandlers["chown"] = onNewBasenameApproversWrapper(model.FileChownEventType)
	AllApproversHandlers["link"] = onNewTwoBasenamesApproversWrapper(model.FileLinkEventType, "file", "file.destination")
	AllApproversHandlers["mkdir"] = onNewBasenameApproversWrapper(model.FileMkdirEventType)
	AllApproversHandlers["open"] = openOnNewApprovers
	AllApproversHandlers["rename"] = onNewTwoBasenamesApproversWrapper(model.FileRenameEventType, "file", "file.destination")
	AllApproversHandlers["rmdir"] = onNewBasenameApproversWrapper(model.FileRmdirEventType)
	AllApproversHandlers["unlink"] = onNewBasenameApproversWrapper(model.FileUnlinkEventType)
	AllApproversHandlers["utimes"] = onNewBasenameApproversWrapper(model.FileUtimesEventType)
	AllApproversHandlers["mmap"] = mmapOnNewApprovers
	AllApproversHandlers["mprotect"] = mprotectOnNewApprovers
	AllApproversHandlers["splice"] = spliceOnNewApprovers
}
