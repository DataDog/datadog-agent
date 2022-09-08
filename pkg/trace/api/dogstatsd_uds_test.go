// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
//
//go:build !windows
// +build !windows

package api

import (
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func TestDogStatsDReverseProxyEndToEndUDS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	dir := t.TempDir()
	socket := filepath.Join(dir, "dsd.socket")
	cfg := config.New()
	cfg.StatsdSocket = socket
	testDogStatsDReverseProxyEndToEnd(t, cfg)
}
