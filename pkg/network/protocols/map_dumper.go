// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package protocols

import (
	"fmt"
	"github.com/cilium/ebpf"
	"io"
)

// WriteMapDumpHeader writes a header for a map dump
func WriteMapDumpHeader(w io.Writer, mapObj *ebpf.Map, mapName string, key interface{}, value interface{}) {
	_, _ = io.WriteString(w, fmt.Sprintf("Map: %q, type: %s, key: %T (%d bytes), value: %T (%d bytes)\n", mapName, mapObj.Type(), key, mapObj.KeySize(), value, mapObj.ValueSize()))
}
