// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package submitterimpl

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/process/forwarders/forwardersimpl"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo/hostinfoimpl"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestSubmitterLifecycle(t *testing.T) {
	_ = fxutil.Test[submitter.Component](t, fx.Options(
		hostinfoimpl.MockModule(),
		core.MockBundle(),
		forwardersimpl.MockModule(),
		Module(),
	))
}
