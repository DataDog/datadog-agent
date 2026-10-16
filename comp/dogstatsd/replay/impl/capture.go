// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package replayimpl

import (
	"context"
	"errors"
	"os"
	"path"
	"time"

	"github.com/spf13/afero"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const captureShutdownTimeout = 5 * time.Second

type Requires struct {
	Lc     compdef.Lifecycle
	Config configComponent.Component
	Tagger tagger.Component
}

// trafficCapture allows capturing traffic from our listeners and writing it to file
type trafficCapture struct {
	writer *TrafficCaptureWriter
	config model.Reader
}

//nolint:revive // TODO(AML) Fix revive linter
func NewComponent(deps Requires) replay.Component {
	tc := &trafficCapture{
		writer: NewTrafficCaptureWriter(deps.Config.GetInt("dogstatsd_capture_depth"), deps.Tagger),
		config: deps.Config,
	}
	deps.Lc.Append(compdef.Hook{
		OnStop: tc.shutdown,
	})

	return tc
}

// shutdown drains an ongoing capture so the file is not left truncated.
func (tc *trafficCapture) shutdown(_ context.Context) error {
	tc.writer.StopAndWait(captureShutdownTimeout)
	return nil
}

// IsOngoing returns whether a capture is ongoing for this TrafficCapture instance.
func (tc *trafficCapture) IsOngoing() bool {
	return tc.writer.IsOngoing()
}

// StartCapture starts a TrafficCapture and returns an error in the event of an issue.
func (tc *trafficCapture) StartCapture(p string, d time.Duration, compressed bool) (string, error) {
	// Cheap rejection; startCapture below is the authoritative reservation.
	if tc.writer.IsOngoing() {
		return "", errors.New("Ongoing capture in progress")
	}

	target, path, err := OpenFile(afero.NewOsFs(), p, tc.defaultlocation())
	if err != nil {
		return "", err
	}

	if _, err := tc.writer.startCapture(target, d, compressed); err != nil {
		// Still ours: nothing else will write to or close it.
		target.Close()
		if rmErr := os.Remove(path); rmErr != nil {
			log.Warnf("could not remove unused capture file %v: %v", path, rmErr)
		}
		return "", err
	}

	return path, nil
}

// StopCapture stops an ongoing TrafficCapture.
func (tc *trafficCapture) StopCapture() {
	tc.writer.StopCapture()
}

// RegisterSharedPoolManager registers the shared pool manager with the TrafficCapture.
func (tc *trafficCapture) RegisterSharedPoolManager(p *packets.PoolManager[packets.Packet]) error {
	return tc.writer.RegisterSharedPoolManager(p)
}

// RegisterOOBPoolManager registers the OOB shared pool manager with the TrafficCapture.
func (tc *trafficCapture) RegisterOOBPoolManager(p *packets.PoolManager[[]byte]) error {
	return tc.writer.RegisterOOBPoolManager(p)
}

// Enqueue enqueues a capture buffer so it's written to file.
func (tc *trafficCapture) Enqueue(msg *replay.CaptureBuffer) bool {
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
	return nil
}
