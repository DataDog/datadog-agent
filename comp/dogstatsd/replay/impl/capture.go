// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package replayimpl

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/spf13/afero"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

//nolint:revive // TODO(AML) Fix revive linter
type Requires struct {
	Lc     compdef.Lifecycle
	Config configComponent.Component
	Tagger tagger.Component
}

// trafficCapture allows capturing traffic from our listeners and writing it to file
type trafficCapture struct {
	writer       *TrafficCaptureWriter
	config       model.Reader
	tagger       tagger.Component
	startUpError error

	sync.RWMutex
}

//nolint:revive // TODO(AML) Fix revive linter
func NewTrafficCapture(deps Requires) replay.Component {
	tc := &trafficCapture{
		config: deps.Config,
		tagger: deps.Tagger,
	}
	deps.Lc.Append(compdef.Hook{
		OnStart: tc.configure,
	})

	return tc
}

func (tc *trafficCapture) configure(_ context.Context) error {
	writer := NewTrafficCaptureWriter(tc.config.GetInt("dogstatsd_capture_depth"), tc.tagger)
	if writer == nil {
		tc.startUpError = fmt.Errorf("unable to instantiate capture writer")
	}
	tc.writer = writer

	return nil
}

// IsOngoing returns whether a capture is ongoing for this TrafficCapture instance.
func (tc *trafficCapture) IsOngoing() bool {
	tc.RLock()
	defer tc.RUnlock()

	if tc.writer == nil {
		return false
	}

	return tc.writer.IsOngoing()
}

// StartCapture starts a TrafficCapture and returns an error in the event of an issue.
func (tc *trafficCapture) StartCapture(p string, d time.Duration, compressed bool) (string, error) {
	if tc.IsOngoing() {
		return "", fmt.Errorf("Ongoing capture in progress")
	}

	target, path, err := OpenFile(afero.NewOsFs(), p, tc.defaultlocation())
	if err != nil {
		return "", err
	}

	go tc.writer.Capture(target, d, compressed)

	return path, nil
}

// StopCapture stops an ongoing TrafficCapture.
func (tc *trafficCapture) StopCapture() {
	tc.Lock()
	defer tc.Unlock()
	if tc.writer == nil {
		return
	}

	tc.writer.StopCapture()
}

// RegisterSharedPoolManager registers the shared pool manager with the TrafficCapture.
func (tc *trafficCapture) RegisterSharedPoolManager(p *packets.PoolManager[packets.Packet]) error {
	tc.Lock()
	defer tc.Unlock()
	return tc.writer.RegisterSharedPoolManager(p)
}

// RegisterOOBPoolManager registers the OOB shared pool manager with the TrafficCapture.
func (tc *trafficCapture) RegisterOOBPoolManager(p *packets.PoolManager[[]byte]) error {
	tc.Lock()
	defer tc.Unlock()
	return tc.writer.RegisterOOBPoolManager(p)
}

// Enqueue enqueues a capture buffer so it's written to file.
func (tc *trafficCapture) Enqueue(msg *replay.CaptureBuffer) bool {
	tc.RLock()
	defer tc.RUnlock()
	return tc.writer.Enqueue(msg)
}

func (tc *trafficCapture) defaultlocation() string {
	location := tc.config.GetString("dogstatsd_capture_path")
	if location == "" {
		location = path.Join(tc.config.GetString("run_path"), "dsd_capture")
	}
	return location
}

func (tc *trafficCapture) GetStartUpError() error {
	return tc.startUpError
}
