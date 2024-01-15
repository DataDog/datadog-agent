// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package manager

import (
	"regexp"

	"github.com/DataDog/gopsutil/process"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type processes map[int32]*process.FilledProcess

var (
	defaultRegexps = []*regexp.Regexp{
		regexp.MustCompile("pause|s6-svscan|s6-supervise"),
		regexp.MustCompile("agent|process-agent|trace-agent|security-agent|system-probe"),
	}
	processFetcher = fetchProcesses
)

func fetchProcesses() (processes, error) {
	return process.AllProcesses()
}

type noProcessExit struct {
	excludedProcesses []*regexp.Regexp
}

// NoProcessExit creates a shutdown detector based on running processes
func NoProcessExit(r []*regexp.Regexp) ExitDetector {
	return &noProcessExit{excludedProcesses: r}
}

// DefaultNoProcessExit creates the default NoProcess shutdown detector
func DefaultNoProcessExit(cfg config.Reader) (ExitDetector, error) {
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

	return NoProcessExit(mergedRegexps), nil
}

func (s *noProcessExit) check() bool {
	processes, err := processFetcher()
	if err != nil {
		log.Debugf("Unable to get processes list to trigger autoexit, err: %v", err)
		return false
	}

	for _, p := range processes {
		isExcluded := false
		for _, r := range s.excludedProcesses {
			if isExcluded = r.MatchString(p.Name); isExcluded {
				break
			}
		}

		if !isExcluded {
			log.Debugf("Processes preventing autoexit: p: %d - %s", p.Pid, p.Name)
			return false
		}
	}

	return true
}
