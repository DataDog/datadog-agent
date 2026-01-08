// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package forwarder

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(defaultforwarder.Params{}),
		core.MockBundle(),
		// TODO: Remove this once the mockForwarder is a proper mock
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
	)
}
