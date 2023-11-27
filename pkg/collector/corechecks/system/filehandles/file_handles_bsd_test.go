// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build freebsd

package filehandles

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

func GetInt64(name string) (value int64, err error) {
	value = 65534
	err = nil
	return
}

func TestFhCheckFreeBSD(t *testing.T) {
	getInt64 = GetInt64

	fileHandleCheck := new(fhCheck)
	fileHandleCheck.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	mock := mocksender.NewMockSender(fileHandleCheck.ID())

	mock.On("Gauge", "system.fs.file_handles.used", 421, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.fs.file_handles.max", 65534, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	fileHandleCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 2)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
