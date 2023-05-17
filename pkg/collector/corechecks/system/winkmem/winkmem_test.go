// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.
//go:build windows

package winkmem

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

func TestWinKMem(t *testing.T) {
	kcheck := new(KMemCheck)
	kcheck.Configure(integration.FakeConfigHash, nil, nil, "test")

	m := mocksender.NewMockSender(kcheck.ID())

	// since we're using the default config, there should
	// be the default number
	m.On("Gauge", "winkmem.paged_pool_bytes", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.On("Gauge", "winkmem.nonpaged_pool_bytes", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.On("Gauge", "winkmem.paged_allocs_outstanding", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.On("Gauge", "winkmem.nonpaged_allocs_outstanding", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.On("Commit").Return().Times(1)

	kcheck.Run()
	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, "Commit", 1)
}
