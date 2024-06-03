// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Factory encapsulates the initialization of a Module
type Factory struct {
	Name             sysconfigtypes.ModuleName
	ConfigNamespaces []string
	Fn               func(cfg *sysconfigtypes.Config, wmeta optional.Option[workloadmeta.Component], telemetry telemetry.Component) (Module, error)
	NeedsEBPF        func() bool
}
