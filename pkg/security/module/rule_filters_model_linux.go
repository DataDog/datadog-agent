// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

type RuleFilterEvent struct {
	*kernel.Version
}

type RuleFilterModel struct {
	*kernel.Version
}

func NewRuleFilterModel() *RuleFilterModel {
	kv, _ := kernel.NewKernelVersion()
	return &RuleFilterModel{
		Version: kv,
	}
}

func (m *RuleFilterModel) NewEvent() eval.Event {
	return &RuleFilterEvent{
		Version: m.Version,
	}
}

func (m *RuleFilterModel) GetEvaluator(field eval.Field, regID eval.RegisterID) (eval.Evaluator, error) {
	switch field {
	case "kernel.version.major":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				kv := ctx.Event.(*RuleFilterEvent)
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
				kv := ctx.Event.(*RuleFilterEvent)
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
				kv := ctx.Event.(*RuleFilterEvent)
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
				kv := ctx.Event.(*RuleFilterEvent)
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
				kv := ctx.Event.(*RuleFilterEvent)
				if ubuntuKernelVersion := kv.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
					return ubuntuKernelVersion.Flavor
				}
				return ""
			},
			Field: field,
		}, nil
	case "os":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return runtime.GOOS },
			Field:   field,
		}, nil
	case "os.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return ctx.Event.(*RuleFilterEvent).OsRelease["ID"] },
			Field:   field,
		}, nil
	case "os.platform_id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return ctx.Event.(*RuleFilterEvent).OsRelease["PLATFORM_ID"] },
			Field:   field,
		}, nil
	case "os.version_id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string { return ctx.Event.(*RuleFilterEvent).OsRelease["VERSION_ID"] },
			Field:   field,
		}, nil

	case "os.is_amazon_linux":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).IsAmazonLinuxKernel() },
			Field:   field,
		}, nil
	case "os.is_cos":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).IsCOSKernel() },
			Field:   field,
		}, nil
	case "os.is_debian":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).IsDebianKernel() },
			Field:   field,
		}, nil
	case "os.is_oracle":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).IsOracleUEKKernel() },
			Field:   field,
		}, nil
	case "os.is_rhel":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool {
				return ctx.Event.(*RuleFilterEvent).IsRH7Kernel() || ctx.Event.(*RuleFilterEvent).IsRH8Kernel()
			},
			Field: field,
		}, nil
	case "os.is_rhel7":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).IsRH7Kernel() },
			Field:   field,
		}, nil
	case "os.is_rhel8":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).IsRH8Kernel() },
			Field:   field,
		}, nil
	case "os.is_sles":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).IsSLESKernel() },
			Field:   field,
		}, nil
	case "os.is_sles12":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).IsSuse12Kernel() },
			Field:   field,
		}, nil
	case "os.is_sles15":
		return &eval.BoolEvaluator{
			EvalFnc: func(ctx *eval.Context) bool { return ctx.Event.(*RuleFilterEvent).IsSuse15Kernel() },
			Field:   field,
		}, nil
	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *RuleFilterEvent) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {
	case "kernel.version.major":
		if ubuntuKernelVersion := e.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
			return int(ubuntuKernelVersion.Major), nil
		}
		return int(e.Code.Major()), nil
	case "kernel.version.minor":
		if ubuntuKernelVersion := e.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
			return int(ubuntuKernelVersion.Minor), nil
		}
		return int(e.Code.Minor()), nil
	case "kernel.version.patch":
		if ubuntuKernelVersion := e.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
			return int(ubuntuKernelVersion.Patch), nil
		}
		return int(e.Code.Patch()), nil
	case "kernel.version.abi":
		if ubuntuKernelVersion := e.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
			return int(ubuntuKernelVersion.Abi), nil
		}
		return 0, nil
	case "kernel.version.flavor":
		if ubuntuKernelVersion := e.UbuntuKernelVersion(); ubuntuKernelVersion != nil {
			return ubuntuKernelVersion.Flavor, nil
		}
		return "", nil

	case "os":
		return runtime.GOOS, nil
	case "os.id":
		return e.OsRelease["ID"], nil
	case "os.platform_id":
		return e.OsRelease["PLATFORM_ID"], nil
	case "os.version_id":
		return e.OsRelease["VERSION_ID"], nil

	case "os.is_amazon_linux":
		return e.IsAmazonLinuxKernel(), nil
	case "os.is_cos":
		return e.IsCOSKernel(), nil
	case "os.is_debian":
		return e.IsDebianKernel(), nil
	case "os.is_oracle":
		return e.IsOracleUEKKernel(), nil
	case "os.is_rhel":
		return e.IsRH7Kernel() || e.IsRH8Kernel(), nil
	case "os.is_rhel7":
		return e.IsRH7Kernel(), nil
	case "os.is_rhel8":
		return e.IsRH8Kernel(), nil
	case "os.is_sles":
		return e.IsSLESKernel(), nil
	case "os.is_sles12":
		return e.IsSuse12Kernel(), nil
	case "os.is_sles15":
		return e.IsSuse15Kernel(), nil
	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}
