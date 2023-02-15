// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/shirou/gopsutil/v3/process"
)

// getFileStats returns the number of files the Agent process has open
func GetFileStats() map[string]interface{} {
	stats := make(map[string]interface{})
	// Creating a new process.Process type based on Agent PID
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		log.Errorf("Failed to retrieve agent process: %s", err)
		return stats
	}

	// Retrieving []OpenFilesStat from Agent process.Process type
	files, err := p.OpenFiles()
	if err != nil {
		log.Errorf("Failed to retrieve agent process' open files slice: %s", err)
		return stats
	}

	// Retrieving number of open files by getting the length of the Agent process.Process type's []OpenFilesStat slice
	stats["Agent Process Open Files"] = strconv.Itoa(len(files))

	// Retrieving type []RlimitStat from type process.Process p
	rs, err := p.Rlimit()
	if err != nil {
		log.Errorf("Failed to retrieve type RlimitStat: %s", err)
		return stats
	}

	// Retrieving RLIMIT_NOFILE (index 7) from Agent process' RLimit[]
	openFilesMaxJSON := []byte(rs[7].String())
	openFilesMax := make(map[string]interface{})
	json.Unmarshal(openFilesMaxJSON, &openFilesMax)
	stats["OS File Limit"] = openFilesMax["soft"]

	return stats
}

func CheckFileStats(stats map[string]interface{}) {
	openFiles, err := GetFloat(stats["Agent Process Open Files"])
	if err != nil {
		log.Errorf("Failed to convert to float: %s", err)
		return
	}

	openFilesMax, err := GetFloat(stats["OS File Limit"])
	if err != nil {
		log.Errorf("Failed to convert to float: %s", err)
		return
	}

	// Log a warning if the ratio between the Agent's open files to the OS file limit is > 0.9, log an error if OS file limit is reached
	if openFiles/openFilesMax > 0.9 {
		log.Warnf("Agent process is close to OS file limit of %v. Agent process currently has %v files open.", openFilesMax, openFiles)
	} else if openFiles/openFilesMax > 1 {
		log.Errorf("Agent process is reaching OS open file limit: %v. This may be preventing log files from being tailed by the Agent. Consider increasing OS file limit.", openFilesMax)
	}
	log.Debugf("Agent process currently has %v files open. OS file limit is currently set to %v.", openFiles, openFilesMax)
}

func GetFloat(unk interface{}) (float64, error) {
	var floatType = reflect.TypeOf(float64(0))
	var stringType = reflect.TypeOf("")

	switch i := unk.(type) {
	case float64:
		return i, nil
	case float32:
		return float64(i), nil
	case int64:
		return float64(i), nil
	case int32:
		return float64(i), nil
	case int:
		return float64(i), nil
	case uint64:
		return float64(i), nil
	case uint32:
		return float64(i), nil
	case uint:
		return float64(i), nil
	case string:
		return strconv.ParseFloat(i, 64)
	default:
		v := reflect.ValueOf(unk)
		v = reflect.Indirect(v)
		if v.Type().ConvertibleTo(floatType) {
			fv := v.Convert(floatType)
			return fv.Float(), nil
		} else if v.Type().ConvertibleTo(stringType) {
			sv := v.Convert(stringType)
			s := sv.String()
			return strconv.ParseFloat(s, 64)
		} else {
			return math.NaN(), fmt.Errorf("Can't convert %v to float64", v.Type())
		}
	}
}
