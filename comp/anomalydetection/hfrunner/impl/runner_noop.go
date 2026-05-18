// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

package hfrunnerimpl

import (
	"sync"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

type runner struct {
	stopCh   chan struct{}
	stopOnce sync.Once
}

func (r *runner) start() {}

func (r *runner) stop() { r.stopOnce.Do(func() { close(r.stopCh) }) }

func newRunner(_ observerdef.Handle) *runner {
	return &runner{stopCh: make(chan struct{})}
}

func newContainerRunner(_ observerdef.Handle, _ ContainerDeps) *runner {
	return nil
}
