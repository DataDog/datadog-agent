// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package collectors

import (
	"errors"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// ErrNothingYet is returned when no collector is currently detected.
// This might change in the future if new collectors are valid.
var ErrNothingYet = &retry.Error{
	LogicError:    errors.New("No collector detected"),
	RessourceName: "detector",
	RetryStatus:   retry.FailWillRetry,
}

// ErrPermaFail is returned when all candidates have permanently failed.
var ErrPermaFail = &retry.Error{
	LogicError:    errors.New("No collector available"),
	RessourceName: "detector",
	RetryStatus:   retry.PermaFail,
}

// Detector holds the logic to initialise collectors, with retries,
// and selecting the most appropriate among available collectors
type Detector struct {
	candidates         map[string]Collector
	detected           map[string]Collector
	preferredCollector Collector
	preferredName      string
}

// NewDetector returns a Detector ready to use. If configuredName
// is empty, autodetection is enabled. If not, only the one name
// will be tried.
func NewDetector(configuredName string) *Detector {
	d := &Detector{
		candidates: make(map[string]Collector),
		detected:   make(map[string]Collector),
	}
	// Load candidates from catalog
	for n, f := range defaultCatalog {
		if configuredName != "" && n != configuredName {
			// If a collector name is configured, skip the others
			continue
		}
		d.candidates[n] = f()
	}
	return d
}

// GetPreferred detects, ranks and returns the best collector for now.
// Result might change if new collectors are valid after start, then
// constant when all collectors are either ok or PermaFail.
// Users should keep calling this method instead of caching the first result.
func (d *Detector) GetPreferred() (Collector, string, error) {
	// Detection rounds finished, exit fast
	if d.candidates == nil {
		if d.preferredCollector == nil {
			return nil, "", ErrPermaFail
		}
		return d.preferredCollector, d.preferredName, nil
	}

	// Retry all remaining candidates
	detected, remaining := retryCandidates(d.candidates)
	d.candidates = remaining

	// Add newly detected detected
	for name, c := range detected {
		d.detected[name] = c
	}

	// Pick preferred collector among detected ones
	preferred := rankCollectors(d.detected, d.preferredName)
	if preferred != d.preferredName {
		log.Infof("Using collector %s", preferred)
		d.preferredName = preferred
		d.preferredCollector = d.detected[preferred]
	}

	// Stop detection when all candidates are tested
	// return a PermaFail error if nothing was detected
	if len(remaining) == 0 {
		d.candidates = nil
		d.detected = nil
		if d.preferredCollector == nil {
			return nil, "", ErrPermaFail
		}
	}

	if d.preferredCollector == nil {
		return nil, "", ErrNothingYet
	}
	return d.preferredCollector, d.preferredName, nil
}

// retryCandidates iterates over candidates and returns two new maps:
// the successfully detected collectors, and the ones to retry later
func retryCandidates(candidates map[string]Collector) (map[string]Collector, map[string]Collector) {
	detected := make(map[string]Collector)
	remaining := make(map[string]Collector)

	for name, c := range candidates {
		err := c.Detect()
		if retry.IsErrWillRetry(err) {
			log.Debugf("Will retry collector %s later: %s", name, err)
			remaining[name] = c
			continue
		}
		if err != nil {
			log.Debugf("Collector %s failed to detect: %s", name, err)
			continue
		}
		log.Infof("Collector %s successfully detected", name)
		detected[name] = c
	}
	return detected, remaining
}

// rankCollectors returns the preferred collector out of a map.
// It ranks by collector priority, then name for stability
func rankCollectors(collectors map[string]Collector, current string) string {
	preferred := current
	for name := range collectors {
		// First one
		if preferred == "" {
			preferred = name
			continue
		}
		if isPrefered(name, preferred) {
			preferred = name
		}
	}
	return preferred
}

// isPrefered compares a collector by name to the current preferred
// to determine whether it should be used instead
func isPrefered(name, current string) bool {
	if collectorPriorities[name] == collectorPriorities[current] {
		// Alphabetic order if priorities are identical
		return strings.Compare(current, name) == 1
	}
	return collectorPriorities[name] > collectorPriorities[current]
}
