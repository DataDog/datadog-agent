// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package api

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/stretchr/testify/assert"
)

func TestHTTPReceiverStart(t *testing.T) {
	var logs bytes.Buffer
	old := log.SetLogger(log.NewBufferLogger(&logs))
	defer log.SetLogger(old)

	for name, tt := range map[string]struct {
		port int      // receiver port to configure the test with
		pipe string   // socket & windows pipe to configure the test with
		out  []string // expected log output (uses strings.Contains)
	}{
		"off": {
			out: []string{"HTTP Server is off: all listeners are disabled"},
		},
		"tcp": {
			port: 8129,
			out: []string{
				"Listening for traces at http://localhost:8129",
			},
		},
		"pipe": {
			pipe: "\\c\\agent.pipe",
			out: []string{
				"HTTP receiver disabled by config (apm_config.receiver_port: 0)",
				`Listening for traces on Windows pipe "\\\\.\\pipe\\\\c\\agent.pipe". Security descriptor is "D:AI(A;;GA;;;WD)"`,
			},
		},
		"both": {
			port: 8129,
			pipe: "\\c\\agent.pipe",
			out: []string{
				"Listening for traces at http://localhost:8129",
				`Listening for traces on Windows pipe "\\\\.\\pipe\\\\c\\agent.pipe". Security descriptor is "D:AI(A;;GA;;;WD)"`,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			logs.Reset()
			cfg := config.New()
			cfg.ReceiverPort = tt.port
			cfg.WindowsPipeName = tt.pipe
			r := newTestReceiverFromConfig(cfg)
			r.Start()
			defer r.Stop()
			for _, l := range tt.out {
				assert.Contains(t, logs.String(), l)
			}
		})
	}
}
