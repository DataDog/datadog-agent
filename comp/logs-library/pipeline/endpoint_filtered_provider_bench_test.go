// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pipeline

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func BenchmarkEndpointFilterPolicyEvaluate(b *testing.B) {
	us1 := config.NewEndpoint("k1", "", "us1.example.com", 443, "", true)
	us5 := config.NewMockEndpointWithOptions(map[string]any{
		"api_key":     "k5",
		"host":        "us5.example.com",
		"port":        443,
		"use_ssl":     true,
		"is_reliable": false,
	})
	eps := config.NewEndpointsWithBatchSettings(us1, []config.Endpoint{us5}, false, true, time.Second, 0, 100, 1024*1024, 10)
	noticeOnly := routing.SelectorFunc(func(v routing.LogView) (bool, error) {
		return v.Status() == message.StatusNotice || v.Status() == message.StatusError, nil
	})
	policy, err := routing.NewEndpointFilterPolicy(eps, []routing.EndpointSelector{nil, noticeOnly})
	if err != nil {
		b.Fatal(err)
	}

	src := sources.NewLogSource("app", &config.LogsConfig{Service: "app", Source: "go", Tags: []string{"env:prod"}})
	infoMsg := message.NewMessage([]byte("info line"), message.NewOrigin(src), message.StatusInfo, 0)
	noticeMsg := message.NewMessage([]byte("notice line"), message.NewOrigin(src), message.StatusNotice, 0)

	b.Run("sparse_miss", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, _ = policy.Evaluate(infoMsg)
		}
	})
	b.Run("match", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, _ = policy.Evaluate(noticeMsg)
		}
	})
}
