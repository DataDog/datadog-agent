// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package configstreambootstrap

import (
	"testing"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// UseDynamicSchema makes the global config auto-rebuild the env layer when any DD_ var changes.
func UseDynamicSchema(t testing.TB) {
	t.Helper()
	pkgconfigsetup.Datadog().SetTestOnlyDynamicSchema(true)
	t.Cleanup(func() { pkgconfigsetup.Datadog().SetTestOnlyDynamicSchema(false) })
}
