// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package uprobe

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	manager "github.com/DataDog/ebpf-manager"
)

type uprobe struct {
	desc model.UProbeDesc
	id   uint64
	uid  string
}

var uprobes = make(map[uint64]*uprobe)

var (
	ErrUProbeRuleMissingPath                 = errors.New("uprobe rule is missing a path value")
	ErrUProbeRuleInvalidPath                 = errors.New("uprobe rule has invalid path value")
	ErrUProbeRuleInvalidVersion              = errors.New("uprobe rule has invalid version value")
	ErrUProbeRuleInvalidFunctionName         = errors.New("uprobe rule has invalid function name value")
	ErrUProbeRuleInvalidOffset               = errors.New("uprobe rule has invalid offset value")
	ErrUProbeRuleMissingFunctionNameOrOffset = errors.New("uprobe rule requires either a function name or an offset value")
)

func GetUProbeDesc(id uint64) *model.UProbeDesc {
	if up, exists := uprobes[id]; exists {
		return &up.desc
	}
	return nil
}

func CreateUProbeFromRule(m *manager.Manager, id uint64, rule *rules.Rule) error {
	pathValues := rule.GetFieldValues("uprobe.path")
	if len(pathValues) == 0 {
		return ErrUProbeRuleMissingPath
	}
	pathValue, ok := pathValues[0].Value.(string)
	if !ok {
		return ErrUProbeRuleInvalidPath
	}

	var versionValue string
	versionValues := rule.GetFieldValues("uprobe.version")
	if len(versionValues) != 0 {
		versionValue, ok = versionValues[0].Value.(string)
		if !ok {
			return ErrUProbeRuleInvalidVersion
		}
	}

	var functionNameValue string
	functionNameValues := rule.GetFieldValues("uprobe.function_name")
	if len(functionNameValues) != 0 {
		functionNameValue, ok = functionNameValues[0].Value.(string)
		if !ok {
			return ErrUProbeRuleInvalidFunctionName
		}
	}

	var offsetValue string
	var offsetInt uint64
	offsetValues := rule.GetFieldValues("uprobe.offset")
	if len(offsetValues) != 0 {
		offsetValue, ok = offsetValues[0].Value.(string)
		if !ok {
			return ErrUProbeRuleInvalidOffset
		}
		var err error
		offsetInt, err = strconv.ParseUint(offsetValue, 0, 64)
		if err != nil {
			return ErrUProbeRuleInvalidOffset
		}
	}

	if len(functionNameValue) == 0 && len(offsetValue) == 0 {
		return ErrUProbeRuleMissingFunctionNameOrOffset
	}

	up := uprobe{
		id:  id,
		uid: fmt.Sprintf("vuln_detector_%d", id),
		desc: model.UProbeDesc{
			Path:         pathValue,
			Version:      versionValue,
			FunctionName: functionNameValue,
			OffsetStr:    offsetValue,
			Offset:       offsetInt,
		},
	}

	err := attachProbe(m, &up)
	if err != nil {
		return err
	}
	uprobes[up.id] = &up

	return nil
}
