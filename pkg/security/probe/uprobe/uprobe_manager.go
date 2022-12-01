// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package uprobe

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	manager "github.com/DataDog/ebpf-manager"
)

type uprobe struct {
	desc model.UProbeDesc
	id   uint64
	pID  manager.ProbeIdentificationPair
}

var allUProbes = make(map[uint64]*uprobe)         // id to uprobe map
var ruleFilesUprobes = make(map[string][]*uprobe) // path to list of uprobe map
var containerUProbes = make(map[uint32][]*uprobe) // container pid one to uprobe map
var m *manager.Manager

var (
	ErrUProbeRuleMissingPath                 = errors.New("uprobe rule is missing a path value")
	ErrUProbeRuleInvalidPath                 = errors.New("uprobe rule has invalid path value")
	ErrUProbeRuleInvalidVersion              = errors.New("uprobe rule has invalid version value")
	ErrUProbeRuleInvalidFunctionName         = errors.New("uprobe rule has invalid function name value")
	ErrUProbeRuleInvalidOffset               = errors.New("uprobe rule has invalid offset value")
	ErrUProbeRuleMissingFunctionNameOrOffset = errors.New("uprobe rule requires either a function name or an offset value")
)

func Init(manager *manager.Manager) {
	m = manager
}

func getNextID() uint64 {
	return uint64(time.Now().UnixNano())
}

func GetUProbeDesc(id uint64) *model.UProbeDesc {
	if up, exists := allUProbes[id]; exists {
		return &up.desc
	}
	return nil
}

func CreateUProbeFromRule(rule *rules.Rule) error {
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

	up := &uprobe{
		id: getNextID(),
		desc: model.UProbeDesc{
			Path:         pathValue,
			Version:      versionValue,
			FunctionName: functionNameValue,
			OffsetStr:    offsetValue,
			Offset:       offsetInt,
		},
	}

	err := attachProbe(m, up)
	if err != nil {
		return err
	}

	allUProbes[up.id] = up
	ruleFilesUprobes[up.desc.Path] = append(ruleFilesUprobes[up.desc.Path], up)

	return nil
}

func GetActivatedProbes() []manager.ProbesSelector {

	selector := &manager.BestEffort{}

	for _, up := range allUProbes {
		selector.Selectors = append(selector.Selectors, &manager.ProbeSelector{
			ProbeIdentificationPair: up.pID,
		})
	}

	return []manager.ProbesSelector{selector}
}

func HandleNewMountNamespace(event *model.NewMountNSEvent) {
	rootPath := utils.RootPath(int32(event.PidOne))

	for path, uProbesForPath := range ruleFilesUprobes {
		fullPath := filepath.Join(rootPath, path)
		fInfo, err := os.Stat(fullPath)
		if err != nil || fInfo.IsDir() {
			continue
		}

		for _, up := range uProbesForPath {
			newUprobe := &uprobe{
				id: getNextID(),
				desc: model.UProbeDesc{
					Path:         fullPath,
					Version:      up.desc.Version,
					FunctionName: up.desc.FunctionName,
					OffsetStr:    up.desc.OffsetStr,
					Offset:       up.desc.Offset,
				},
			}

			err := attachProbe(m, newUprobe)
			if err != nil {
				seclog.Errorf("failed to attach container uprobe %s/%s:%s err: %w", newUprobe.pID.UID, newUprobe.desc.Path, newUprobe.desc.FunctionName, err)
				continue
			}

			allUProbes[up.id] = newUprobe
			containerUProbes[event.PidOne] = append(containerUProbes[event.PidOne], newUprobe)

			seclog.Infof("attached uprobe %s/%s:%s", newUprobe.pID.UID, newUprobe.desc.Path, newUprobe.desc.FunctionName)
		}
	}
}

func HandleProcessExit(event *model.ExitEvent) {
	for _, up := range containerUProbes[event.PIDContext.Tid] {
		if err := m.DetachHook(up.pID); err != nil {
			seclog.Warnf("failed to detach uprobe %s/%s:%s err: %w", up.pID.UID, up.desc.Path, up.desc.FunctionName, err)
		} else {
			seclog.Infof("detached uprobe %s/%s:%s", up.pID.UID, up.desc.Path, up.desc.FunctionName)
		}
		delete(allUProbes, up.id)
	}
	delete(containerUProbes, event.PIDContext.Tid)
}
