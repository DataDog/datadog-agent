// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package forwarder

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	secretsfxmock "github.com/DataDog/datadog-agent/comp/core/secrets/fx-mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(defaultforwarder.Params{}),
		core.MockBundle(),
		secretsfxmock.MockModule(),
	)
}
