// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package replay

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/fx"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	// GUID will be used as the GUID during capture replays
	// This is a magic number chosen for no particular reason other than the fact its
	// quite large an improbable to match an actual Group ID on any given box. We
	// need this number to identify replayed Unix socket ancillary credentials.
	GUID = 999888777
)

type dependencies struct {
	fx.In

	Config configComponent.Component
}

// TrafficCapture allows capturing traffic from our listeners and writing it to file
type trafficCapture struct {
	writer *TrafficCaptureWriter
	config config.Reader

	sync.RWMutex
}

// TODO: (components) - remove once serverless is an FX app
//
//nolint:revive // TODO(AML) Fix revive linter
func NewServerlessTrafficCapture() Component {
	return newTrafficCaptureCompat(config.Datadog)
}

// TODO: (components) - merge with newTrafficCaptureCompat once NewServerlessTrafficCapture is removed
func newTrafficCapture(deps dependencies) Component {
	panic("not called")
}

func newTrafficCaptureCompat(cfg config.Reader) Component {
	return &trafficCapture{
		config: cfg,
	}
}

func (tc *trafficCapture) Configure() error {
	writer := NewTrafficCaptureWriter(tc.config.GetInt("dogstatsd_capture_depth"))
	if writer == nil {
		return fmt.Errorf("unable to instantiate capture writer")
	}
	tc.writer = writer

	return nil
}

// IsOngoing returns whether a capture is ongoing for this TrafficCapture instance.
func (tc *trafficCapture) IsOngoing() bool {
	panic("not called")
}

// Start starts a TrafficCapture and returns an error in the event of an issue.
func (tc *trafficCapture) Start(p string, d time.Duration, compressed bool) (string, error) {
	panic("not called")
}

// Stop stops an ongoing TrafficCapture.
func (tc *trafficCapture) Stop() {
	panic("not called")
}

// RegisterSharedPoolManager registers the shared pool manager with the TrafficCapture.
func (tc *trafficCapture) RegisterSharedPoolManager(p *packets.PoolManager) error {
	tc.Lock()
	defer tc.Unlock()
	return tc.writer.RegisterSharedPoolManager(p)
}

// RegisterOOBPoolManager registers the OOB shared pool manager with the TrafficCapture.
func (tc *trafficCapture) RegisterOOBPoolManager(p *packets.PoolManager) error {
	tc.Lock()
	defer tc.Unlock()
	return tc.writer.RegisterOOBPoolManager(p)
}

// Enqueue enqueues a capture buffer so it's written to file.
func (tc *trafficCapture) Enqueue(msg *CaptureBuffer) bool {
	panic("not called")
}

func (tc *trafficCapture) defaultlocation() string {
	panic("not called")
}
