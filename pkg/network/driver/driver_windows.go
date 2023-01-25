// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package driver

import (
	"errors"
	"fmt"
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

// ErrDriverNotInitialized is returned when you attempt to use the driver without calling Init
var ErrDriverNotInitialized = errors.New("driver has not been initialized")

// ErrClosedSourceNotAllowed is returned if you attempt to use the driver and closed source is not allowed
var ErrClosedSourceNotAllowed = errors.New("closed source driver is not allowed")

var driverInit sync.Once
var driverRef *driver

type driver struct {
	inuse   *atomic.Uint32
	allowed bool
}

// Init configures the driver and will disable it if closed source is not allowed
func Init(cfg *config.Config) error {
	driverInit.Do(func() {
		driverRef = &driver{
			inuse:   atomic.NewUint32(0),
			allowed: cfg.ClosedSourceAllowed,
		}
	})
	return nil
}

// Start will start the driver if this is the first user
func Start() error {
	if driverRef == nil {
		return ErrDriverNotInitialized
	}
	return driverRef.start()
}

// Stop will stop the driver if this is the last user
func Stop() error {
	if driverRef == nil {
		return ErrDriverNotInitialized
	}
	if driverRef.inuse.Load() == 0 {
		return fmt.Errorf("driver.Stop called without corresponding Start")
	}
	return driverRef.stop(false)
}

// ForceStop forcefully stops the driver without concern to current usage
func ForceStop() error {
	if driverRef == nil {
		return ErrDriverNotInitialized
	}
	return driverRef.stop(true)
}

// IsNeeded will return if one or more users have called Start and not called Stop yet
func IsNeeded() bool {
	if driverRef == nil {
		return false
	}
	return driverRef.isNeeded()
}

func (d *driver) isNeeded() bool {
	if !d.allowed {
		return false
	}
	return d.inuse.Load() > 0
}

func (d *driver) start() error {
	if !d.allowed {
		return ErrClosedSourceNotAllowed
	}
	if refs := d.inuse.Inc(); refs == 1 {
		return startDriverService(driverServiceName)
	}
	return nil
}

func (d *driver) stop(force bool) error {
	if force {
		d.inuse.Store(0)
		return stopDriverService(driverServiceName, !d.allowed)
	}
	if refs := d.inuse.Dec(); refs == 0 {
		return stopDriverService(driverServiceName, false)
	}
	return nil
}
