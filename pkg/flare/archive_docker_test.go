// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package flare

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrimCommand(t *testing.T) {
	for in, out := range map[string]string{
		"/pause":                 "/pause",
		"nginx -g 'daemon off;'": "nginx …",
		"/entrypoint.sh datadog-cluster-agent start": "/entrypoint.sh …",
		"/coredns -conf /etc/coredns/Corefile":       "/coredns …",
		"/my/very/long/command":                      "/my/very/long/command",
		"/my/very/very/very/very/very/long/command":  "/my/very/very/very/very/very/…",
	} {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, out, trimCommand(in))
		})
	}
}
