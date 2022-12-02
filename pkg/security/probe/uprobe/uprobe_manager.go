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
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	manager "github.com/DataDog/ebpf-manager"
)

const DefaultMaxConcurrentUProbes = uint64(100)

var (
	ErrUProbeRuleMissingPath                 = errors.New("uprobe rule is missing a path value")
	ErrUProbeRuleInvalidPath                 = errors.New("uprobe rule has invalid path value")
	ErrUProbeRuleInvalidVersion              = errors.New("uprobe rule has invalid version value")
	ErrUProbeRuleInvalidFunctionName         = errors.New("uprobe rule has invalid function name value")
	ErrUProbeRuleInvalidOffset               = errors.New("uprobe rule has invalid offset value")
	ErrUProbeRuleMissingFunctionNameOrOffset = errors.New("uprobe rule requires either a function name or an offset value")
	ErrMaxConcurrentUProbes                  = errors.New("max concurrent uprobe reached")
)

type UProbeManagerOptions struct {
	MaxConcurrentUProbes uint64
}

var uman *uprobeManager

type uprobeManager struct {
	lock             sync.Mutex
	options          UProbeManagerOptions
	allUProbes       map[uint64]*uprobe   // id to uprobe map
	ruleFilesUprobes map[string][]*uprobe // path to list of uprobe map
	containerUProbes map[uint32][]*uprobe // container pid one to uprobe map
	m                *manager.Manager
	uprobeFreeList   chan *uprobe
	nextRuleID       uint64
}

type uprobe struct {
	desc   model.UProbeDesc
	id     uint64
	ruleID uint64
	pID    manager.ProbeIdentificationPair
}

func Init(manager *manager.Manager, options UProbeManagerOptions) {
	if uman == nil {
		uman = &uprobeManager{
			allUProbes:       make(map[uint64]*uprobe),
			ruleFilesUprobes: make(map[string][]*uprobe),
			containerUProbes: make(map[uint32][]*uprobe),
			m:                manager,
			options:          options,
		}

		if options.MaxConcurrentUProbes == 0 {
			uman.options.MaxConcurrentUProbes = DefaultMaxConcurrentUProbes
		}
		uman.uprobeFreeList = make(chan *uprobe, uman.options.MaxConcurrentUProbes)
		for i := uint64(0); i < uman.options.MaxConcurrentUProbes; i++ {
			putUProbe(&uprobe{id: i})
		}
	}
}

func getNextRuleID() uint64 {
	id := uman.nextRuleID
	uman.nextRuleID++
	return id
}

func getUProbe() *uprobe {
	select {
	case up := <-uman.uprobeFreeList:
		return up
	default:
		return nil
	}
}

func putUProbe(up *uprobe) {
	if up != nil {
		uman.uprobeFreeList <- up
	}
}

func GetUProbeDesc(id uint64) *model.UProbeDesc {
	uman.lock.Lock()
	defer uman.lock.Unlock()

	if up, exists := uman.allUProbes[id]; exists {
		return &up.desc
	}
	return nil
}

func CreateUProbeFromRule(rule *rules.Rule) error {
	uman.lock.Lock()
	defer uman.lock.Unlock()

	up := getUProbe()
	if up == nil {
		return ErrMaxConcurrentUProbes
	}

	pathValues := rule.GetFieldValues("uprobe.path")
	if len(pathValues) == 0 {
		putUProbe(up)
		return ErrUProbeRuleMissingPath
	}
	pathValue, ok := pathValues[0].Value.(string)
	if !ok {
		putUProbe(up)
		return ErrUProbeRuleInvalidPath
	}

	var versionValue string
	versionValues := rule.GetFieldValues("uprobe.version")
	if len(versionValues) != 0 {
		versionValue, ok = versionValues[0].Value.(string)
		if !ok {
			putUProbe(up)
			return ErrUProbeRuleInvalidVersion
		}
	}

	var functionNameValue string
	functionNameValues := rule.GetFieldValues("uprobe.function_name")
	if len(functionNameValues) != 0 {
		functionNameValue, ok = functionNameValues[0].Value.(string)
		if !ok {
			putUProbe(up)
			return ErrUProbeRuleInvalidFunctionName
		}
	}

	var offsetValue string
	var offsetInt uint64
	offsetValues := rule.GetFieldValues("uprobe.offset")
	if len(offsetValues) != 0 {
		offsetValue, ok = offsetValues[0].Value.(string)
		if !ok {
			putUProbe(up)
			return ErrUProbeRuleInvalidOffset
		}
		var err error
		offsetInt, err = strconv.ParseUint(offsetValue, 0, 64)
		if err != nil {
			putUProbe(up)
			return ErrUProbeRuleInvalidOffset
		}
	}

	if len(functionNameValue) == 0 && len(offsetValue) == 0 {
		putUProbe(up)
		return ErrUProbeRuleMissingFunctionNameOrOffset
	}

	up.desc.Path = pathValue
	up.desc.Version = versionValue
	up.desc.FunctionName = functionNameValue
	up.desc.OffsetStr = offsetValue
	up.desc.Offset = offsetInt
	up.ruleID = getNextRuleID()

	err := attachProbe(uman.m, up)
	if err != nil {
		putUProbe(up)
		return err
	}

	uman.allUProbes[up.id] = up
	uman.ruleFilesUprobes[up.desc.Path] = append(uman.ruleFilesUprobes[up.desc.Path], up)

	return nil
}

func GetActivatedProbes() []manager.ProbesSelector {
	uman.lock.Lock()
	defer uman.lock.Unlock()

	selector := &manager.BestEffort{}

	for _, up := range uman.allUProbes {
		selector.Selectors = append(selector.Selectors, &manager.ProbeSelector{
			ProbeIdentificationPair: up.pID,
		})
	}

	return []manager.ProbesSelector{selector}
}

func HandleNewMountNamespace(event *model.NewMountNSEvent) error {
	uman.lock.Lock()
	defer uman.lock.Unlock()

	rootPath := utils.RootPath(int32(event.PidOne))

	for path, uProbesForPath := range uman.ruleFilesUprobes {
		fullPath := filepath.Join(rootPath, path)
		fInfo, err := os.Stat(fullPath)
		if err != nil || fInfo.IsDir() {
			continue
		}

		for _, up := range uProbesForPath {

			newUProbe := getUProbe()
			if newUProbe == nil {
				return ErrMaxConcurrentUProbes
			}

			newUProbe.desc.Path = fullPath
			newUProbe.desc.Version = up.desc.Version
			newUProbe.desc.FunctionName = up.desc.FunctionName
			newUProbe.desc.OffsetStr = up.desc.OffsetStr
			newUProbe.desc.Offset = up.desc.Offset
			newUProbe.ruleID = up.ruleID

			err := attachProbe(uman.m, newUProbe)
			if err != nil {
				putUProbe(newUProbe)
				seclog.Errorf("failed to attach container uprobe %s %s:%s err: %w", newUProbe.pID.UID, newUProbe.desc.Path, newUProbe.desc.FunctionName, err)
				continue
			}

			uman.allUProbes[up.id] = newUProbe
			uman.containerUProbes[event.PidOne] = append(uman.containerUProbes[event.PidOne], newUProbe)

			seclog.Infof("attached uprobe %s %s:%s", newUProbe.pID.UID, newUProbe.desc.Path, newUProbe.desc.FunctionName)
		}
	}

	return nil
}

func HandleProcessExit(event *model.ExitEvent) {
	uman.lock.Lock()
	defer uman.lock.Unlock()

	for _, up := range uman.containerUProbes[event.PIDContext.Tid] {
		if err := uman.m.DetachHook(up.pID); err != nil {
			seclog.Warnf("failed to detach uprobe %s %s:%s err: %w", up.pID.UID, up.desc.Path, up.desc.FunctionName, err)
		} else {
			seclog.Infof("detached uprobe %s %s:%s", up.pID.UID, up.desc.Path, up.desc.FunctionName)
		}
		delete(uman.allUProbes, up.id)
		putUProbe(up)
	}
	delete(uman.containerUProbes, event.PIDContext.Tid)
}
