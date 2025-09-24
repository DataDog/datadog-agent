// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	manager "github.com/DataDog/ebpf-manager"
)

type probeSelectorBuilder struct {
	uid string
}

type psbOption func(*probeSelectorBuilder)

func hookFunc(funcName string, options ...psbOption) *manager.ProbeSelector {
	psb := &probeSelectorBuilder{
		uid: SecurityAgentUID,
	}

	for _, opt := range options {
		opt(psb)
	}

	return &manager.ProbeSelector{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          psb.uid,
			EBPFFuncName: funcName,
		},
	}
}

func withUID(uid string) psbOption {
	return func(psb *probeSelectorBuilder) {
		psb.uid = uid
	}
}
