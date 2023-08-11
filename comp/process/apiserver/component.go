// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package apiserver initializes the api server that powers many subcommands.
package apiserver

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: processes

// Component exported type should have comment or be unexported
type Component interface {
}

// Module exported var should have comment or be unexported
var Module = fxutil.Component(
	fx.Provide(newApiServer),
)
