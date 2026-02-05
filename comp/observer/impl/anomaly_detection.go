// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package observerimpl

import (
	"strings"
	"sync"
	"time"

	logger "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly"
)

type AnomalyDetection struct {
	log              logger.Component
	profileProcessor *anomaly.ProfileProcessor
	profileBuffer    [][]byte
	profileMutex     sync.Mutex
	stopChan         chan struct{}
	wg               sync.WaitGroup
}

func NewAnomalyDetection(log logger.Component) *AnomalyDetection {
	a := &AnomalyDetection{
		log:              log,
		profileProcessor: anomaly.NewProfileProcessor(),
		profileBuffer:    make([][]byte, 0),
		stopChan:         make(chan struct{}),
	}

	// Start the background processing goroutine
	a.wg.Add(1)
	go a.processProfilesPeriodically()

	return a
}

// Stop gracefully stops the background processing
func (a *AnomalyDetection) Stop() {
	close(a.stopChan)
	a.wg.Wait()
}

func (a *AnomalyDetection) ProcessMetric(metric *metricObs) {
	if !strings.HasPrefix(metric.name, "datadog") {
		a.log.Debugf("Processing metric: %v", metric)
	}
}

func (a *AnomalyDetection) ProcessLog(log *logObs) {
	a.log.Debugf("Processing log: %v", log)
}

func (a *AnomalyDetection) ProcessTrace(trace *traceObs) {
	a.log.Debugf("Processing trace: %v", trace)
}

func (a *AnomalyDetection) ProcessProfile(profile *profileObs) {
	if profile.profileType == "go" {
		// Store the profile in the buffer
		a.profileMutex.Lock()
		a.profileBuffer = append(a.profileBuffer, profile.rawData)
		a.profileMutex.Unlock()

		a.log.Debugf("Stored profile in buffer (total: %d)", len(a.profileBuffer))
	}
	a.log.Debugf("Processing profile: %v", profile)
}

// processProfilesPeriodically drains and processes profiles every 10 seconds
func (a *AnomalyDetection) processProfilesPeriodically() {
	defer a.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.drainAndProcessProfiles()
		case <-a.stopChan:
			// Final drain before stopping
			a.drainAndProcessProfiles()
			return
		}
	}
}

// drainBuffer drains the profile buffer and returns all accumulated profiles
func (a *AnomalyDetection) drainBuffer() [][]byte {
	a.profileMutex.Lock()
	defer a.profileMutex.Unlock()

	if len(a.profileBuffer) == 0 {
		return nil
	}

	profiles := make([][]byte, len(a.profileBuffer))
	copy(profiles, a.profileBuffer)
	a.profileBuffer = a.profileBuffer[:0]
	return profiles
}

// drainAndProcessProfiles processes all accumulated profiles and clears the buffer
func (a *AnomalyDetection) drainAndProcessProfiles() {
	profiles := a.drainBuffer()
	if len(profiles) == 0 {
		return
	}

	a.log.Infof("Processing %d accumulated profiles", len(profiles))

	topFuncs, err := a.profileProcessor.GetTopFunctions(profiles, 10)
	if err != nil {
		a.log.Warnf("Failed to process accumulated profiles: %v", err)
		return
	}

	if len(topFuncs.CPU) > 0 {
		a.log.Infof("Top 10 CPU consuming functions (from %d profiles):", len(profiles))
		for i, fn := range topFuncs.CPU {
			a.log.Infof("  %d. %s: %d samples", i+1, fn.Name, fn.Flat)
		}
	}

	if len(topFuncs.Memory) > 0 {
		a.log.Infof("Top 10 memory consuming functions (from %d profiles):", len(profiles))
		for i, fn := range topFuncs.Memory {
			a.log.Infof("  %d. %s: %d bytes", i+1, fn.Name, fn.Bytes)
		}
	}
}
