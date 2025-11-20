// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package ebpf holds ebpf related files
package ebpf

import (
	ddebpfmaps "github.com/DataDog/datadog-agent/pkg/ebpf/maps"
)

// BatchAPISupported indicates whether the maps batch API is supported
var BatchAPISupported = ddebpfmaps.BatchAPISupported
