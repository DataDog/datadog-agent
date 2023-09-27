// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package log

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func TestMockLogging(t *testing.T) {
	log := fxutil.Test[Component](t, fx.Options(
		fx.Supply(Params{}),
		config.MockModule,
		MockModule,
	))
	log.Debugf("hello, world. %s", "hi")
}
