// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ciscosdwan

import (
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
	"testing"
)

type deps struct {
	fx.In
	Demultiplexer demultiplexer.Mock
}

func createDeps(t *testing.T) deps {
	return fxutil.Test[deps](t, demultiplexerimpl.MockModule(), defaultforwarder.MockModule(), core.MockBundle())
}

func Test_check_viptela(t *testing.T) {
}
