// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"context"
	"runtime"
	"testing"
	"time"

	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

func BenchmarkBuffer(b *testing.B) {
	var buffer *ConnectionBuffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer := NewConnectionBuffer(256, 256)
		for i := 0; i < 512; i++ {
			conn := buffer.Next()
			conn.Pid = uint32(i)
		}
	}
	runtime.KeepAlive(buffer)
}

func TestPrintAzureMetadata(t *testing.T) {
	metadataURL := "http://169.254.169.254"
	ctx := context.Background()
	timeout := 5 * time.Second

	fullMetadata, err := httputils.Get(ctx, metadataURL+"/metadata/instance?api-version=2017-04-02&format=json", map[string]string{"Metadata": "true"}, timeout)
	if err != nil {
		t.Log("fullMetadata err ", err)
	}
	t.Log(fullMetadata)

	networkMetadata, err := httputils.Get(ctx, metadataURL+"/metadata/instance/network?api-version=2017-04-02&format=json", map[string]string{"Metadata": "true"}, timeout)
	if err != nil {
		t.Log("networkMetadata err ", err)
	}
	t.Log(networkMetadata)

	interfaceMetadata, err := httputils.Get(ctx, metadataURL+"/metadata/instance/network/interface?api-version=2017-04-02&format=json", map[string]string{"Metadata": "true"}, timeout)
	if err != nil {
		t.Log("interfaceMetadata err ", err)
	}
	t.Log(interfaceMetadata)

	interface0Metadata, err := httputils.Get(ctx, metadataURL+"/metadata/instance/network/interface/0?api-version=2017-04-02&format=json", map[string]string{"Metadata": "true"}, timeout)
	if err != nil {
		t.Log("interface0Metadata err ", err)
	}
	t.Log(interface0Metadata)

	publicIpAddress, err := httputils.Get(ctx, metadataURL+"/metadata/instance/network/interface/0/ipv4/ipAddress/0/publicIpAddress?api-version=2017-04-02&format=text", map[string]string{"Metadata": "true"}, timeout)
	if err != nil {
		t.Log("publicIpAddress err ", err)
	}
	t.Log(publicIpAddress)
}
