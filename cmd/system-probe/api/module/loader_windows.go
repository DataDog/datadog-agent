// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func preRegister(cfg *config.Config) error {
	if err := driver.Init(cfg); err != nil {
		return fmt.Errorf("failed to load driver subsystem: %v", err)
	}
	return nil
}

func postRegister(_ *config.Config) error {
	if !driver.IsNeeded() {
		// if running, shut it down
		log.Debug("Shutting down the driver.  Upon successful initialization, it was not needed by the current configuration.")

		// shut the driver down and  disable it
		if err := driver.ForceStop(); err != nil {
			log.Warnf("error stopping driver: %s", err)
		}
	}
	return nil
}
