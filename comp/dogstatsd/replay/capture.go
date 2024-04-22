// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package replay

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/spf13/afero"
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

	Lc     fx.Lifecycle
	Config configComponent.Component
}

// TrafficCapture allows capturing traffic from our listeners and writing it to file
type trafficCapture struct {
	writer       *TrafficCaptureWriter
	config       config.Reader
	startUpError error

	sync.RWMutex
}

// TODO: (components) - remove once serverless is an FX app
//
//nolint:revive // TODO(AML) Fix revive linter
func NewServerlessTrafficCapture() Component {
	tc := newTrafficCaptureCompat(config.Datadog)
	_ = tc.configure(context.TODO())
	return tc
}

// TODO: (components) - merge with newTrafficCaptureCompat once NewServerlessTrafficCapture is removed
func newTrafficCapture(deps dependencies) Component {
	tc := newTrafficCaptureCompat(deps.Config)
	deps.Lc.Append(fx.Hook{
		OnStart: tc.configure,
	})

	return tc
}

func newTrafficCaptureCompat(cfg config.Reader) *trafficCapture {
	return &trafficCapture{
		config: cfg,
	}
}

func (tc *trafficCapture) configure(_ context.Context) error {
	writer := NewTrafficCaptureWriter(tc.config.GetInt("dogstatsd_capture_depth"))
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
