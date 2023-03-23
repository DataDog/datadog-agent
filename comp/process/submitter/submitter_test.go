// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package submitter

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestSubmitterLifecycle(t *testing.T) {
	fxutil.Test(t, fx.Options(
		hostinfo.MockModule,
		core.MockBundle,
		Module,
	), func(runner Component) {
		// Start and stop the component
	})
}
