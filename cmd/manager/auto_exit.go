// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package manager

import (
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/net/context"
)

const (
	defaultExitTicker = 30 * time.Second
)

// ExitDetector is common interface for shutdown mechanisms
type ExitDetector interface {
	check() bool
}

// ConfigureAutoExit starts automatic shutdown mechanism if necessary
func ConfigureAutoExit(ctx context.Context) error {
	var sd ExitDetector
	var err error

	if config.Datadog.GetBool("auto_exit.noprocess.enabled") {
		sd, err = DefaultNoProcessExit()
	}

	if err != nil {
		return err
	}

	if sd == nil {
		return nil
	}

	validationPeriod := time.Duration(config.Datadog.GetInt("auto_exit.validation_period")) * time.Second
	return startAutoExit(ctx, sd, defaultExitTicker, validationPeriod)
}

func startAutoExit(ctx context.Context, sd ExitDetector, tickerPeriod, validationPeriod time.Duration) error {
	if sd == nil {
		return fmt.Errorf("a shutdown detector must be provided")
	}

	selfProcess, err := os.FindProcess(os.Getpid())
	if err != nil {
		return fmt.Errorf("cannot find own process, err: %w", err)
	}

	log.Info("Starting auto-exit watcher")
	lastConditionNotMet := time.Now()
	ticker := time.NewTicker(tickerPeriod)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				shutdownConditionFound := sd.check()
				if shutdownConditionFound {
					if lastConditionNotMet.Add(validationPeriod).Before(time.Now()) {
						log.Info("Conditions met for automatic exit: triggering stop sequence")
						if err := selfProcess.Signal(os.Interrupt); err != nil {
							log.Errorf("Unable to send termination signal - will use os.exit, err: %v", err)
							os.Exit(1)
						}
						return
					}
				} else {
					lastConditionNotMet = time.Now()
				}
			}
		}
	}()

	return nil
}
