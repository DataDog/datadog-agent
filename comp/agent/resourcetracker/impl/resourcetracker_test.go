// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package resourcetrackerimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type submitterMock struct {
	mock.Mock
}

func (m *submitterMock) Gauge(name string, value float64, tags []string) {
	m.Called(name, value, tags)
}

func TestResourceTracker(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	log := logmock.New(t)
	submitter := &submitterMock{}
	requires := Requires{
		Lifecycle: lc,
		Log:       log,
		Submitter: submitter,
	}
	rt, err := NewComponent(requires)
	assert.NotNil(t, rt.Comp)
	assert.NoError(t, err)

	pid := os.Getpid()
	process, err := os.Executable()
	assert.NoError(t, err)
	process = filepath.Base(process)
	tags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("process:%s", process),
	}
	submitter.On("Gauge", "datadog.agent.process.cpu", mock.Anything, tags).Return()
	submitter.On("Gauge", "datadog.agent.process.rss", mock.Anything, tags).Return()

	err = lc.Start(context.Background())
	assert.NoError(t, err)
	assert.Eventually(t, func() bool { return len(submitter.Calls) >= 2 }, 5*time.Second, 100*time.Millisecond)
	submitter.AssertExpectations(t)
	err = lc.Stop(context.Background())
	assert.NoError(t, err)
}
