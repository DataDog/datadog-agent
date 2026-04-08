// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && linux_bpf

package module

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestDiscovery(t *testing.T) {
	t.Run("rust", func(t *testing.T) {
		suite.Run(t, &discoveryTestSuite{setupModule: setupRustDiscoveryModule})
	})
}
