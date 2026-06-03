// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package jobmetadatademo

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestRunEmitsCurrentTaggerMetadataTags(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, "demo-container"),
		"dogstatsd-gpu-job-metadata",
		nil,
		[]string{"gpu_job_id:job-a", "team:ml"},
		nil,
		nil,
	)

	check := newCheck(fakeTagger).(*Check)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := check.Configure(senderManager, integration.FakeConfigHash, []byte("container_id: demo-container\nlog_results: false\n"), nil, "test", "provider")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(check.ID(), senderManager)
	mockSender.SetupAcceptAll()

	err = check.Run()
	require.NoError(t, err)

	mockSender.AssertMetric(t, "Gauge", metricName, 1.0, "", []string{"container_id:demo-container", "gpu_job_id:job-a", "team:ml"})
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}
