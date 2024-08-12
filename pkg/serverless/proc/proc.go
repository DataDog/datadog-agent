// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package proc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ProcStatPath           = "/proc/stat"
	ProcUptimePath         = "/proc/uptime"
	ProcNetDevPath         = "/proc/net/dev"
	ProcSysFsFilenrPath    = "/proc/sys/fs/file-nr"
	lambdaNetworkInterface = "vinternal_1"
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
	var user, nice, system, idle, iowait, irq, softirq, steal, guest, guestNice float64
	_, err = fmt.Fscanln(file, &label, &user, &nice, &system, &idle, &iowait, &irq, &softirq, &steal, &guest, &guestNice)
	if err != nil {
		return nil, err
	}

	// SC_CLK_TCK is the system clock frequency in ticks per second
	// We'll use this to convert CPU times from user HZ to milliseconds
	clcktck, err := getClkTck()
	if err != nil {
		return nil, err
	}

	cpuData.TotalUserTimeMs = (1000 * user) / float64(clcktck)
	cpuData.TotalSystemTimeMs = (1000 * system) / float64(clcktck)
	cpuData.TotalIdleTimeMs = (1000 * idle) / float64(clcktck)

	// Scan for cpuN lines
	perCPUDataMap := map[string]float64{}
	for {
		_, err = fmt.Fscanln(file, &label, &user, &nice, &system, &idle, &iowait, &irq, &softirq, &steal, &guest, &guestNice)
		if err != nil && !strings.HasPrefix(label, "cpu") {
			break
		} else if err != nil {
			return nil, err
		}
		perCPUDataMap[label] = (1000 * idle) / float64(clcktck)
	}
	if len(perCPUDataMap) == 0 {
		return nil, fmt.Errorf("per-core CPU data not found in file %s", path)
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

type NetworkData struct {
	RxBytes float64
	TxBytes float64
}

// GetNetworkData collects bytes sent and received by the function
func GetNetworkData() (*NetworkData, error) {
	return getNetworkData(ProcNetDevPath)
}

func getNetworkData(path string) (*NetworkData, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var interfaceName string
	var rxBytes, rxPackets, rxErrs, rxDrop, rxFifo, rxFrame, rxCompressed, rxMulticast, txBytes,
		txPackets, txErrs, txDrop, txFifo, txColls, txCarrier, txCompressed float64
	for {
		_, err = fmt.Fscanln(file, &interfaceName, &rxBytes, &rxPackets, &rxErrs, &rxDrop, &rxFifo, &rxFrame,
			&rxCompressed, &rxMulticast, &txBytes, &txPackets, &txErrs, &txDrop, &txFifo, &txColls, &txCarrier,
			&txCompressed)
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("network data not found in file '%s'", path)
		}
		if err == nil && strings.HasPrefix(interfaceName, lambdaNetworkInterface) {
			return &NetworkData{
				RxBytes: rxBytes,
				TxBytes: txBytes,
			}, nil
		}
	}

}

type FileDescriptorData struct {
	AllocatedFileHandles float64
	UnusedFileHandles    float64
	MaximumFileHandles   float64
}

// GetNetworkData collects bytes sent and received by the function
func GetFileDescriptorData() (*FileDescriptorData, error) {
	return getFileDescriptorData(ProcSysFsFilenrPath)
}

func getFileDescriptorData(path string) (*FileDescriptorData, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var allocatedFileHandles, unusedFileHandles, maximumFileHandles float64
	for {
		_, err = fmt.Fscanln(file, &allocatedFileHandles, &unusedFileHandles, &maximumFileHandles)
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("file descriptor data not found in file '%s'", path)
		}
		if err == nil {
			return &FileDescriptorData{
				AllocatedFileHandles: allocatedFileHandles,
				UnusedFileHandles:    unusedFileHandles,
				MaximumFileHandles:   maximumFileHandles,
			}, nil
		}
	}

}
