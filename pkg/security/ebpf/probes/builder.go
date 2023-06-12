// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probes

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
)

type probeSelectorBuilder struct {
	uid          string
	skipIfFentry bool
}

type psbOption func(*probeSelectorBuilder)

func kprobeOrFentry(funcName string, fentry bool, options ...psbOption) *manager.ProbeSelector {
	psb := &probeSelectorBuilder{
		uid:          SecurityAgentUID,
		skipIfFentry: false,
	}

	for _, opt := range options {
		opt(psb)
	}

	if fentry && psb.skipIfFentry {
		return nil
	}

	return &manager.ProbeSelector{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          psb.uid,
			EBPFFuncName: fmt.Sprintf("hook_%s", funcName),
		},
	}
}

func withSkipIfFentry(skip bool) psbOption {
	return func(psb *probeSelectorBuilder) {
		psb.skipIfFentry = skip
	}
}
