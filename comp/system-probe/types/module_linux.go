// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"

type SystemProbeModuleComponent interface {
	Name() types.ModuleName
	ConfigNamespaces() []string
	Create() (SystemProbeModule, error)
	NeedsEBPF() bool
	OptionalEBPF() bool
}
