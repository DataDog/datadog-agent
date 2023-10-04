// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostinfo wraps the hostinfo inside a component. This is useful because it is relied on by other components.
package hostinfo

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: processes

type Component interface {
	Object() *checks.HostInfo
}

var Module = fxutil.Component(
	fx.Provide(newHostInfo),
)

var MockModule = fxutil.Component(
	fx.Provide(newMockHostInfo),
)
