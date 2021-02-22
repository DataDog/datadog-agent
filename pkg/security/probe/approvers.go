// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"path"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type onApproverHandler func(probe *Probe, approvers rules.Approvers) (activeApprovers, error)
type activeApprover = activeKFilter
type activeApprovers = activeKFilters

var allApproversHandlers = make(map[eval.EventType]onApproverHandler)

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

func onNewBasenameApprovers(probe *Probe, eventType model.EventType, field string, approvers rules.Approvers) ([]activeApprover, error) {
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
		case prefix + ".basename":
			activeApprovers, err := approveBasenames("basename_approvers", eventType, stringValues(values)...)
			if err != nil {
				return nil, err
			}
			basenameApprovers = append(basenameApprovers, activeApprovers...)

		case prefix + ".filename":
			for _, value := range stringValues(values) {
				basename := path.Base(value)
				activeApprover, err := approveBasename("basename_approvers", eventType, basename)
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
	return func(probe *Probe, approvers rules.Approvers) (activeApprovers, error) {
		basenameApprovers, err := onNewBasenameApprovers(probe, event, "", approvers)
		if err != nil {
			return nil, err
		}
		return newActiveKFilters(basenameApprovers...), nil
	}
}

func onNewTwoBasenamesApproversWrapper(event model.EventType, field1, field2 string) onApproverHandler {
	return func(probe *Probe, approvers rules.Approvers) (activeApprovers, error) {
		basenameApprovers, err := onNewBasenameApprovers(probe, event, field1, approvers)
		if err != nil {
			return nil, err
		}
		basenameApprovers2, err := onNewBasenameApprovers(probe, event, field2, approvers)
		if err != nil {
			return nil, err
		}
		basenameApprovers = append(basenameApprovers, basenameApprovers2...)
		return newActiveKFilters(basenameApprovers...), nil
	}
}

func init() {
	allApproversHandlers["chmod"] = onNewBasenameApproversWrapper(model.FileChmodEventType)
	allApproversHandlers["chown"] = onNewBasenameApproversWrapper(model.FileChownEventType)
	allApproversHandlers["link"] = onNewTwoBasenamesApproversWrapper(model.FileLinkEventType, "source", "target")
	allApproversHandlers["mkdir"] = onNewBasenameApproversWrapper(model.FileMkdirEventType)
	allApproversHandlers["open"] = openOnNewApprovers
	allApproversHandlers["rename"] = onNewTwoBasenamesApproversWrapper(model.FileRenameEventType, "old", "new")
	allApproversHandlers["rmdir"] = onNewBasenameApproversWrapper(model.FileRmdirEventType)
	allApproversHandlers["unlink"] = onNewBasenameApproversWrapper(model.FileUnlinkEventType)
	allApproversHandlers["utimes"] = onNewBasenameApproversWrapper(model.FileUtimeEventType)
}
