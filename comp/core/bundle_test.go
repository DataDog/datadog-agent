// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package core

import (
	"fmt"
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	for _, resolveSecrets := range []bool{true, false} {
		t.Run(fmt.Sprintf("resolveSecrets=%t", resolveSecrets), func(t *testing.T) {
			fxutil.TestBundle(t,
				Bundle(resolveSecrets),
				fx.Supply(BundleParams{}),
				fx.Supply(pidimpl.NewParams("")),
			)
		})
	}
}

func TestMockBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, MockBundle(), fx.Supply(BundleParams{}))
}
