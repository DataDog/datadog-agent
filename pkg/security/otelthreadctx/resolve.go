// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelthreadctx

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/otelprocessctx"
)

// Registry stores the layouts by tgid: the eBPF otel_tls map, or the ptracer's
// process cache.
type Registry interface {
	Registered(pid uint32) bool
	Register(pid uint32, layout Layout) error
}

// Step is the step of Resolve an error comes from.
type Step int

// Step values
const (
	StepProcessContext Step = iota
	StepAttributeKeys
	StepLayout
)

func (s Step) String() string {
	switch s {
	case StepProcessContext:
		return "process context"
	case StepAttributeKeys:
		return "attribute keys"
	case StepLayout:
		return "layout"
	}
	return fmt.Sprintf("step %d", int(s))
}

// StepError is an error of Resolve.
type StepError struct {
	Step Step
	Err  error
}

func (e *StepError) Error() string {
	return fmt.Sprintf("%s: %s", e.Step, e.Err)
}

func (e *StepError) Unwrap() error {
	return e.Err
}

// Resolution is what Resolve found out about a process.
type Resolution struct {
	// Nil when the process published none.
	ProcessContext otelprocessctx.ProcessContext
	// Set even when the layout then fails to resolve.
	AttributeKeys []string
	// The ELF analysis ran and succeeded, even if Register then failed.
	LayoutResolved bool
}

// Resolve reads the process context of the process the target was last Reset
// to, and registers its layout unless reg has one: a process republishes on
// every context update, but its layout only changes with exec, so the ELF
// analysis runs once per image. Errors are *StepError.
func (p *Target) Resolve(reg Registry) (Resolution, error) {
	procCtx, err := p.ProcessContext()
	if err != nil {
		return Resolution{}, &StepError{Step: StepProcessContext, Err: err}
	}
	if procCtx == nil {
		return Resolution{}, nil
	}

	res := Resolution{ProcessContext: procCtx}

	res.AttributeKeys, err = AttributeKeys(procCtx)
	if err != nil {
		return res, &StepError{Step: StepAttributeKeys, Err: err}
	}

	if reg.Registered(p.pid) {
		return res, nil
	}

	layout, err := p.ResolveLayout(procCtx)
	if err != nil {
		return res, &StepError{Step: StepLayout, Err: err}
	}
	res.LayoutResolved = true

	if err := reg.Register(p.pid, layout); err != nil {
		return res, &StepError{Step: StepLayout, Err: err}
	}

	return res, nil
}
