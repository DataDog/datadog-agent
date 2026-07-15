// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package agenttelemetryimpl

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// runnerMock is a mock implementation of runner. (TODO: use mock.Mock)
type runnerMock struct {
	mock.Mock
	jobs []job
}

func (r *runnerMock) run() {
	for _, j := range r.jobs {
		j.Run()
	}
}

func (r *runnerMock) start() {
}

func (r *runnerMock) stop() context.Context {
	return context.Background()
}

func (r *runnerMock) addJob(j job) {
	r.jobs = append(r.jobs, j)
}

func newRunnerMock() runner {
	return &runnerMock{}
}
