// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package remoteconfig

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	authtokenmock "github.com/DataDog/datadog-agent/comp/api/authtoken/mock"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(), core.MockBundle(), settingsimpl.MockModule(), fx.Provide(func(t testing.TB) authtoken.Component { return authtokenmock.New(t) }))
}
