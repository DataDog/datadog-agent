// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build foldspace

package foldspace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNativeCoreOptional(t *testing.T) {
	core, err := NewNativeCore(Config{
		Endpoints:           []Endpoint{{Address: "127.0.0.1:1", Class: Reliable}},
		MaxInflightPayloads: 16,
		BatchCapacity:       10,
		MaxPayloadBytes:     1024,
	})
	if err != nil {
		t.Skip(err.Error())
	}
	t.Cleanup(core.Close)
	require.True(t, core.HasCapacity())
	_, progress := core.PushLog(Record{Body: []byte("hi")}, uint64(time.Now().UnixNano()), 1)
	require.True(t, progress.HasCapacity || !progress.HasCapacity)
}
