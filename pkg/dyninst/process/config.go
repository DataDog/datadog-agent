// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package process

import "github.com/DataDog/datadog-agent/pkg/dyninst/ir"

// Config represents the current instrumentation configuration for a process.
// It embeds Info so callers can conveniently access process metadata without
// additional lookups.
type Config struct {
	Info

	RuntimeID         string
	Probes            []ir.ProbeDefinition
	ShouldUploadSymDB bool
}

// ProcessesUpdate aggregates lifecycle removals and refreshed configuration
// for processes discovered by dynamic instrumentation components.
type ProcessesUpdate struct {
	Removals []ID
	Updates  []Config
}
