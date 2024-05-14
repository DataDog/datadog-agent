// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoexitimpl

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	defaultExitTicker = 30 * time.Second
)

type processes map[int32]string

var (
	defaultRegexps = []*regexp.Regexp{
		regexp.MustCompile("pause|s6-svscan|s6-supervise"),
		regexp.MustCompile("agent|process-agent|trace-agent|security-agent|system-probe"),
	}
	processFetcher = fetchProcesses
)

// exitDetector is common interface for shutdown mechanisms
type exitDetector interface {
	check() bool
}

// configureAutoExit starts automatic shutdown mechanism if necessary
func configureAutoExit(ctx context.Context, cfg config.Component, log log.Component) error {
	var sd exitDetector
	var err error

	if cfg.GetBool("auto_exit.noprocess.enabled") {
		sd, err = defaultNoProcessExit(cfg, log)
	}

	if err != nil {
		return err
	}

	if sd == nil {
		return nil
	}

	validationPeriod := time.Duration(cfg.GetInt("auto_exit.validation_period")) * time.Second
	return startAutoExit(ctx, sd, log, defaultExitTicker, validationPeriod)
}

func startAutoExit(ctx context.Context, sd exitDetector, log log.Component, tickerPeriod, validationPeriod time.Duration) error {
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

func fetchProcesses(log log.Component) (processes, error) {
	ps, err := process.Processes()
	if err != nil {
		return nil, err
	}

	procs := make(processes)
	for _, p := range ps {
		name, err := p.Name()
		if err != nil {
			log.Debugf("unable to get process name for PID %d: %s", p.Pid, err)
			continue
		}
		procs[p.Pid] = name
	}
	return procs, nil
}

// defaultNoProcessExit creates the default NoProcess shutdown detector
func defaultNoProcessExit(cfg config.Component, log log.Component) (exitDetector, error) {
	mergedRegexps := make([]*regexp.Regexp, len(defaultRegexps))
	copy(mergedRegexps, defaultRegexps)

	extraRegexps := cfg.GetStringSlice("auto_exit.noprocess.excluded_processes")
	for _, strRegexp := range extraRegexps {
		r, err := regexp.Compile(strRegexp)
		if err != nil {
			return nil, err
		}

		mergedRegexps = append(mergedRegexps, r)
	}

	return &noProcessExit{excludedProcesses: mergedRegexps, log: log}, nil
}

type noProcessExit struct {
	excludedProcesses []*regexp.Regexp
	log               log.Component
}

func (s *noProcessExit) check() bool {
	processes, err := processFetcher(s.log)
	if err != nil {
		s.log.Debugf("Unable to get processes list to trigger autoexit, err: %v", err)
		return false
	}

	for pid, name := range processes {
		isExcluded := false
		for _, r := range s.excludedProcesses {
			if isExcluded = r.MatchString(name); isExcluded {
				break
			}
		}

		if !isExcluded {
			s.log.Debugf("Processes preventing autoexit: p: %d - %s", pid, name)
			return false
		}
	}

	return true
}
