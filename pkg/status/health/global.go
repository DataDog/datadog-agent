// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"encoding/json"
	"errors"
	"expvar"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"time"
)

var readinessAndLivenessCatalog = newCatalog()
var readinessOnlyCatalog = newCatalog()

// RegisterReadiness registers a component for readiness check with the default 30 seconds timeout, returns a token
func RegisterReadiness(name string) *Handle {
	return readinessOnlyCatalog.register(name)
}

// RegisterLiveness registers a component fore liveness check with the default 30 seconds timeout, returns a token
func RegisterLiveness(name string) *Handle {
	return readinessAndLivenessCatalog.register(name)
}

// Deregister a component from the healthcheck
func Deregister(handle *Handle) error {
	if readinessAndLivenessCatalog.deregister(handle) == nil {
		return nil
	}
	return readinessOnlyCatalog.deregister(handle)
}

// GetLive returns health of all components registered for liveness
func GetLive() Status {
	return readinessAndLivenessCatalog.getStatus()
}

// GetReady returns health of all components registered for both readiness and liveness
func GetReady() (ret Status) {
	liveStatus := readinessAndLivenessCatalog.getStatus()
	readyStatus := readinessOnlyCatalog.getStatus()
	ret.Healthy = append(liveStatus.Healthy, readyStatus.Healthy...)
	ret.Unhealthy = append(liveStatus.Unhealthy, readyStatus.Unhealthy...)
	return
}

func GetRunnersStatus(runnersName []string) (ret Status) {
	runnerStatsJSON := []byte(expvar.Get("runner").String())
	runnerStats := make(map[string]interface{})
	err := json.Unmarshal(runnerStatsJSON, &runnerStats)
	if err != nil {
		_ = log.Errorf("Failed to get runners status. %w", err)
		ret.Unhealthy = append(ret.Unhealthy, "agent")
		return
	}

	checks := runnerStats["Checks"].(map[string]interface{})

	for _, runnerName := range runnersName {
		healthy := isRunnerHealthy(checks, runnerName)
		if healthy {
			ret.Healthy = append(ret.Healthy, runnerName)
		} else {
			ret.Unhealthy = append(ret.Unhealthy, runnerName)
		}
	}

	return
}

func isRunnerHealthy(checks map[string]interface{}, runnerName string) bool {
	val, ok := checks[runnerName]
	if !ok {
		_ = log.Errorf("Failed to get runner status. Runner with %s name is not registered", runnerName)
		return false
	}
	runnerInstance := val.(map[string]interface{})
	for _, v := range runnerInstance {
		lastError := v.(map[string]interface{})["LastError"]
		if lastError != "" {
			return false
		}
	}
	return true
}

// todo remove code duplication
func getStatusNonBlockingArguments(getStatus func([]string) Status, args []string) (Status, error) {
	// Run the health status in a goroutine
	ch := make(chan Status, 1)
	go func() {
		ch <- getStatus(args)
	}()

	// Only wait 500ms before returning
	select {
	case status := <-ch:
		return status, nil
	case <-time.After(500 * time.Millisecond):
		return Status{}, errors.New("timeout when getting health status")
	}
}

// getStatusNonBlocking allows to query the health status of the agent
// and is guaranteed to return under 500ms.
func getStatusNonBlocking(getStatus func() Status) (Status, error) {
	// Run the health status in a goroutine
	ch := make(chan Status, 1)
	go func() {
		ch <- getStatus()
	}()

	// Only wait 500ms before returning
	select {
	case status := <-ch:
		return status, nil
	case <-time.After(500 * time.Millisecond):
		return Status{}, errors.New("timeout when getting health status")
	}
}

// GetLiveNonBlocking returns the health of all components registered for liveness with a 500ms timeout
func GetLiveNonBlocking() (Status, error) {
	return getStatusNonBlocking(GetLive)
}

// GetReadyNonBlocking returns the health of all components registered for both readiness and liveness with a 500ms timeout
func GetReadyNonBlocking() (Status, error) {
	return getStatusNonBlocking(GetReady)
}

// todo desc
func GetRunnersNonBlocking(runnerName []string) (Status, error) {
	return getStatusNonBlockingArguments(GetRunnersStatus, runnerName)
}
