// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package proc

import (
	"bufio"
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
	ProcNetDevPath         = "/proc/net/dev"
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

// GetCPUData collects CPU usage data, returning total user CPU time, total system CPU time, error
func GetCPUData(path string) (float64, float64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	readLine, _, err := reader.ReadLine()
	if err != nil {
		return 0, 0, err
	}

	// The first line contains CPU totals aggregated across all CPUs
	lineString := string(readLine)
	cpuTotals := strings.Split(lineString, " ")
	if len(cpuTotals) != 12 {
		return 0, 0, errors.New("incorrect number of columns in file")
	}

	// SC_CLK_TCK is the system clock frequency in ticks per second
	// We'll use this to convert CPU times from user HZ to milliseconds
	clcktck, err := getClkTck()
	if err != nil {
		return 0, 0, err
	}

	userCPUTime, err := strconv.ParseFloat(cpuTotals[2], 64)
	if err != nil {
		return 0, 0, err
	}
	userCPUTimeMs := (1000 * userCPUTime) / float64(clcktck)

	systemCPUTime, err := strconv.ParseFloat(cpuTotals[4], 64)
	if err != nil {
		return 0, 0, err
	}
	systemCPUTimeMs := (1000 * systemCPUTime) / float64(clcktck)

	return userCPUTimeMs, systemCPUTimeMs, nil
}

type NetworkData struct {
	RxBytes float64
	TxBytes float64
}

// GetNetworkData collects bytes sent and received by the function
func GetNetworkData(path string) (*NetworkData, error) {
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
			return nil, errors.New(fmt.Sprintf("network data not found in file '%s'", path))
		}
		if err == nil && strings.HasPrefix(interfaceName, lambdaNetworkInterface) {
			return &NetworkData{
				RxBytes: rxBytes,
				TxBytes: txBytes,
			}, nil
		}
	}

}
