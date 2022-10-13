// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package tracer

import "testing"

func TestProtocolClassification(t *testing.T) {
	cfg := testConfig()
	if !classificationSupported(cfg) {
		t.Skip("Classification is not supported")
	}
	t.Run("without nat", func(t *testing.T) {
		testProtocolClassification(t, cfg, "localhost", "127.0.0.1", "127.0.0.1:0")
	})
}
