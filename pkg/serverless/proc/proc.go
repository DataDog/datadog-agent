// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package proc

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ProcStatPath   = "/proc/stat"
	ProcUptimePath = "/proc/uptime"
)

func getPidList(procPath string) []int {
	files, err := os.ReadDir(procPath)
	pids := []int{}
	if err != nil {
		log.Debug("could not list /proc files")
		return pids
	}
	for _, file := range files {
		if file.IsDir() {
			if processID, err := strconv.Atoi(file.Name()); err == nil {
				pids = append(pids, processID)
			}
		}
	}
	return pids
}

func getEnvVariablesFromPid(procPath string, pid int) map[string]string {
	envVars := map[string]string{}

	bytesRead, err := os.ReadFile(fmt.Sprintf("%s/%d/environ", procPath, pid))
	if err != nil {
		log.Debug("could not list environment variable for proc id %d", pid)
		return envVars
	}

	nullByte := "\000"
	items := bytes.Split(bytesRead, []byte(nullByte))
	for _, item := range items {
		if len(item) > 0 {
			parts := strings.Split(string(item), "=")
			if len(parts) == 2 {
				envVars[parts[0]] = parts[1]
			}
		}
	}

	return envVars
}

// SearchProcsForEnvVariable returns values of the given env variable name
// it returns a slice since a value could be found in more than one process
func SearchProcsForEnvVariable(procPath string, envName string) []string {
	result := []string{}
	pidList := getPidList(procPath)
	for _, pid := range pidList {
		envMap := getEnvVariablesFromPid(procPath, pid)
		if value, ok := envMap[envName]; ok {
			result = append(result, value)
		}
	}
	return result
}

type CPUData struct {
	TotalUserTimeMs   float64
	TotalSystemTimeMs float64
	TotalIdleTimeMs   float64
	// Maps CPU core name (e.g. "cpu1") to time in ms that the CPU spent in the idle process
	IndividualCPUIdleTimes map[string]float64
}

// GetCPUData collects aggregated and per-core CPU usage data
func GetCPUData() (*CPUData, error) {
	return getCPUData(ProcStatPath)
}

func getCPUData(path string) (*CPUData, error) {
	cpuData := CPUData{}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var label string
	var totalUser, totalNice, totalSystem, totalIdle, totalIowait, totalIrq, totalSoftirq, totalSteal, totalGuest, totalGuestNice float64
	_, err = fmt.Fscanln(file, &label, &totalUser, &totalNice, &totalSystem, &totalIdle, &totalIowait, &totalIrq, &totalSoftirq, &totalSteal, &totalGuest, &totalGuestNice)
	if err != nil {
		return nil, err
	}

	// SC_CLK_TCK is the system clock frequency in ticks per second
	// We'll use this to convert CPU times from user HZ to milliseconds
	clcktck, err := getClkTck()
	if err != nil {
		return nil, err
	}

	cpuData.TotalUserTimeMs = (1000 * totalUser) / float64(clcktck)
	cpuData.TotalSystemTimeMs = (1000 * totalSystem) / float64(clcktck)
	cpuData.TotalIdleTimeMs = (1000 * totalIdle) / float64(clcktck)

	// Scan for cpuN lines
	perCPUDataMap := map[string]float64{}
	var perCPULabel string
	var user, nice, system, idle, iowait, irq, softirq, steal, guest, guestNice float64
	for {
		_, err = fmt.Fscanln(file, &perCPULabel, &user, &nice, &system, &idle, &iowait, &irq, &softirq, &steal, &guest, &guestNice)
		if err != nil || !strings.HasPrefix(perCPULabel, "cpu") {
			break
		}
		perCPUDataMap[perCPULabel] = (1000 * idle) / float64(clcktck)
	}
	cpuData.IndividualCPUIdleTimes = perCPUDataMap

	return &cpuData, nil
}

// GetUptime collects uptime data
func GetUptime() (float64, error) {
	return getUptime(ProcUptimePath)
}

func getUptime(path string) (float64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var uptime, idleTime float64
	_, err = fmt.Fscanln(file, &uptime, &idleTime)
	if err != nil {
		return 0, err
	}

	// Convert from seconds to milliseconds
	return uptime * 1000, nil
}
