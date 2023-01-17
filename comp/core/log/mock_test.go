// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestMockLogging(t *testing.T) {
	fxutil.Test(t, fx.Options(
		fx.Supply(Params{}),
		config.MockModule,
		MockModule,
	), func(log Component) {
		log.Debugf("hello, world. %s", "hi")
	})
}
