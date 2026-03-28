// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package fx

import (
	"go.uber.org/fx"

	formatterimpl "github.com/DataDog/datadog-agent/comp/snmptraps/formatter/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule provides a dummy formatter that just hashes packets for testing.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(formatterimpl.NewDummyFormatter),
	)
}
