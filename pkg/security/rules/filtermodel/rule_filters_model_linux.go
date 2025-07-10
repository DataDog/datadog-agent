// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package filtermodel holds rules related files
package filtermodel

import (
	"os"
	"runtime"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// RuleFilterEvent defines a rule filter event
type RuleFilterEvent struct {
	kv  *kernel.Version
	cfg RuleFilterEventConfig
	ipc ipc.Component
}

// RuleFilterModel defines a filter model
type RuleFilterModel struct {
	kv  *kernel.Version
	cfg RuleFilterEventConfig
	ipc ipc.Component
}

// NewRuleFilterModel returns a new rule filter model
func NewRuleFilterModel(cfg RuleFilterEventConfig, ipc ipc.Component) (*RuleFilterModel, error) {
	kv, err := kernel.NewKernelVersion()
	if err != nil {
		return nil, err
	}
	return &RuleFilterModel{
		kv:  kv,
		cfg: cfg,
		ipc: ipc,
	}, nil
}

// NewRuleFilterModelWithKernelVersion returns a new rule filter model
func NewRuleFilterModelWithKernelVersion(cfg RuleFilterEventConfig, kv *kernel.Version) *RuleFilterModel {
	return &RuleFilterModel{
		kv:  kv,
		cfg: cfg,
	}
}

// NewEvent returns a new event
func (m *RuleFilterModel) NewEvent() eval.Event {
	return &RuleFilterEvent{
		kv:  m.kv,
		cfg: m.cfg,
		ipc: m.ipc,
	}
}

// GetEvaluator gets the evaluator
func (m *RuleFilterModel) GetEvaluator(field eval.Field, _ eval.RegisterID, _ int) (eval.Evaluator, error) {
	switch field {
	case "kernel.version.major":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				kv := ctx.Event.(*RuleFilterEvent).kv
				if ubuntuKernelVersion := kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
					return int(ubuntuKernelVersion.Major)
				}
				return int(kv.Code.Major())
			},
			Field: field,
		}, nil
	case "kernel.version.minor":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				kv := ctx.Event.(*RuleFilterEvent).kv
				if ubuntuKernelVersion := kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
					return int(ubuntuKernelVersion.Minor)
				}
				return int(kv.Code.Minor())
			},
			Field: field,
		}, nil
	case "kernel.version.patch":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				kv := ctx.Event.(*RuleFilterEvent).kv
				if ubuntuKernelVersion := kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
					return int(ubuntuKernelVersion.Patch)
				}
				return int(kv.Code.Patch())
			},
			Field: field,
		}, nil
	case "kernel.version.abi":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				kv := ctx.Event.(*RuleFilterEvent).kv
				if ubuntuKernelVersion := kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
					return int(ubuntuKernelVersion.Abi)
				}
				return 0
			},
			Field: field,
		}, nil
	case "kernel.version.flavor":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				kv := ctx.Event.(*RuleFilterEvent).kv
				if ubuntuKernelVersion := kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
					return ubuntuKernelVersion.Flavor
				}
				return ""
			},
			Field: field,
		}, nil
	case "os":
		return &eval.StringEvaluator{
			EvalFnc: func(_ *eval.Context) string { return runtime.GOOS },
			Field:   field,
		}, nil
	case "os.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return ctx.Event.(*RuleFilterEvent).kv.OsRelease["ID"] },
			Field:   field,
		}, nil
	case "os.platform_id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return ctx.Event.(*RuleFilterEvent).kv.OsRelease["PLATFORM_ID"] },
			Field:   field,
		}, nil
	case "os.version_id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return ctx.Event.(*RuleFilterEvent).kv.OsRelease["VERSION_ID"] },
			Field:   field,
		}, nil

	case "os.is_amazon_linux":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).kv.IsAmazonLinuxKernel() },
			Field:   field,
		}, nil
	case "os.is_cos":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).kv.IsCOSKernel() },
			Field:   field,
		}, nil
	case "os.is_debian":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).kv.IsDebianKernel() },
			Field:   field,
		}, nil
	case "os.is_oracle":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).kv.IsOracleUEKKernel() },
			Field:   field,
		}, nil
	case "os.is_rhel":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return ctx.Event.(*RuleFilterEvent).kv.IsRH7Kernel() || ctx.Event.(*RuleFilterEvent).kv.IsRH8Kernel()
			},
			Field: field,
		}, nil
	case "os.is_rhel7":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).kv.IsRH7Kernel() },
			Field:   field,
		}, nil
	case "os.is_rhel8":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).kv.IsRH8Kernel() },
			Field:   field,
		}, nil
	case "os.is_sles":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).kv.IsSLESKernel() },
			Field:   field,
		}, nil
	case "os.is_sles12":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).kv.IsSuse12Kernel() },
			Field:   field,
		}, nil
	case "os.is_sles15":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).kv.IsSuse15Kernel() },
			Field:   field,
		}, nil
	case "envs":
		return &eval.StringArrayEvaluator{
			Values: os.Environ(),
			Field:  field,
		}, nil
	case "origin":
		return &eval.StringEvaluator{
			Value: m.cfg.Origin,
			Field: field,
		}, nil
	case "hostname":
		return &eval.StringEvaluator{
			Value: getHostname(m.ipc),
			Field: field,
		}, nil
	case "kernel.core.enabled":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				revt := ctx.Event.(*RuleFilterEvent)
				return revt.cfg.COREEnabled && revt.kv.SupportCORE()
			},
			Field: field,
		}, nil
	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

// GetFieldValue gets a field value
func (e *RuleFilterEvent) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {
	case "kernel.version.major":
		if ubuntuKernelVersion := e.kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
			return int(ubuntuKernelVersion.Major), nil
		}
		return int(e.kv.Code.Major()), nil
	case "kernel.version.minor":
		if ubuntuKernelVersion := e.kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
			return int(ubuntuKernelVersion.Minor), nil
		}
		return int(e.kv.Code.Minor()), nil
	case "kernel.version.patch":
		if ubuntuKernelVersion := e.kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
			return int(ubuntuKernelVersion.Patch), nil
		}
		return int(e.kv.Code.Patch()), nil
	case "kernel.version.abi":
		if ubuntuKernelVersion := e.kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
			return int(ubuntuKernelVersion.Abi), nil
		}
		return 0, nil
	case "kernel.version.flavor":
		if ubuntuKernelVersion := e.kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
			return ubuntuKernelVersion.Flavor, nil
		}
		return "", nil

	case "os":
		return runtime.GOOS, nil
	case "os.id":
		return e.kv.OsRelease["ID"], nil
	case "os.platform_id":
		return e.kv.OsRelease["PLATFORM_ID"], nil
	case "os.version_id":
		return e.kv.OsRelease["VERSION_ID"], nil

	case "os.is_amazon_linux":
		return e.kv.IsAmazonLinuxKernel(), nil
	case "os.is_cos":
		return e.kv.IsCOSKernel(), nil
	case "os.is_debian":
		return e.kv.IsDebianKernel(), nil
	case "os.is_oracle":
		return e.kv.IsOracleUEKKernel(), nil
	case "os.is_rhel":
		return e.kv.IsRH7Kernel() || e.kv.IsRH8Kernel(), nil
	case "os.is_rhel7":
		return e.kv.IsRH7Kernel(), nil
	case "os.is_rhel8":
		return e.kv.IsRH8Kernel(), nil
	case "os.is_sles":
		return e.kv.IsSLESKernel(), nil
	case "os.is_sles12":
		return e.kv.IsSuse12Kernel(), nil
	case "os.is_sles15":
		return e.kv.IsSuse15Kernel(), nil
	case "envs":
		return os.Environ(), nil
	case "origin":
		return e.cfg.Origin, nil
	case "hostname":
		return getHostname(e.ipc), nil
	case "kernel.core.enabled":
		return e.cfg.COREEnabled && e.kv.SupportCORE(), nil
	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}
