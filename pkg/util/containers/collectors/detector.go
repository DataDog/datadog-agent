// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package collectors

import (
	"errors"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// ErrNothing is returned when no collector is currently available.
// This might change in the future if new collectors are valid.
var ErrNothing = errors.New("No collector available")

// Detector holds the logic to initialise collectors,
// with retries, and selecting the most appropriate
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
	if d.preferredCollector != nil {
		return d.preferredCollector, d.preferredName, nil
	}
	if d.candidates != nil {
		d.detectCandidates()
	}
	return nil, "", ErrNothing
}

func (d *Detector) detectCandidates() {
	// Stop detection when all candidates are tested
	if len(d.candidates) == 0 {
		d.candidates = nil
		d.detected = nil
		return
	}

	foundNew := false
	// Retry all remaining candidates
	for name, c := range d.candidates {
		err := c.Detect()
		if retry.IsErrWillRetry(err) {
			log.Debugf("will retry collector %s later: %s", name, err)
			continue // we want to retry later
		}
		if err != nil {
			log.Debugf("%s collector cannot detect: %s", name, err)
			delete(d.candidates, name)
		} else {
			log.Infof("%s collector successfully detected", name)
			d.detected[name] = c
			foundNew = true
			delete(d.candidates, name)
		}
	}

	// Skip ordering if we have no new collector
	if !foundNew {
		return
	}

	// Pick preferred collector among detected ones
	var preferred string
	for name := range d.detected {
		// First one
		if preferred == "" {
			preferred = name
			continue
		}
		if isPrefered(name, preferred) {
			preferred = name
			continue
		}
	}

	log.Infof("Using collector %s", preferred)
	d.preferredName = preferred
	d.preferredCollector = d.detected[preferred]
}

// isPrefered compares a collector by name to the current preferred
// to determine whether it should be used instead
func isPrefered(name, current string) bool {
	// Highest priority first
	if collectorPriorities[name] > collectorPriorities[current] {
		return true
	}
	if collectorPriorities[name] < collectorPriorities[current] {
		return false
	}
	// Alphabetic order to stay stable
	return strings.Compare(current, name) > 1
}
