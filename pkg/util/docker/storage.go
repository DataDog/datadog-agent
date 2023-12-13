// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ErrStorageStatsNotAvailable is returned if the storage stats are not in the docker info.
	ErrStorageStatsNotAvailable = errors.New("docker storage stats not available")
	diskBytesRe                 = regexp.MustCompile("([0-9.]+)\\s?([a-zA-Z]+)")
	diskUnits                   = map[string]uint64{
		"b":  1,
		"kb": 1000,
		"mb": 1000000,
		"gb": 1000000000,
		"tb": 1000000000000,
	}
)

const (
	// DataStorageName represent diskmapper data stats
	DataStorageName = "data"
	// MetadataStorageName represent diskmapper metadata stats
	MetadataStorageName = "metadata"
)

// StorageStats holds the available stats for a given storage type.
// Non available stats will result in nil pointer, user has to check
// for nil before using the value.
type StorageStats struct {
	Name  string
	Free  *uint64
	Used  *uint64
	Total *uint64
}

// GetPercentUsed computes the used percent (from 0 to 100), even if
// only two of three stats are available. If only one is available
// or total is 0, Nan is returned.
func (s *StorageStats) GetPercentUsed() float64 {
	total := s.Total
	if s.Total != nil && s.Used != nil && s.Free != nil {
		if *s.Total < *s.Used+*s.Free {
			log.Debugf("total lower than free+used, re-computing total")
			totalValue := *s.Used + *s.Free
			total = &totalValue
		}
	}

	if s.Used != nil && total != nil {
		return (100.0 * float64(*s.Used) / float64(*total))
	}
	if s.Free != nil && total != nil {
		return 100.0 - (100.0 * float64(*s.Free) / float64(*total))
	}
	if s.Used != nil && s.Free != nil {
		return (100.0 * float64(*s.Used) / float64(*s.Used+*s.Free))
	}
	return math.NaN()
}

// parseStorageStatsFromInfo converts the [][2]string DriverStatus from docker
// info into a reliable StorageStats struct. It only supports DeviceMapper
// stats for now.
func parseStorageStatsFromInfo(info types.Info) ([]*StorageStats, error) {
	statsArray := []*StorageStats{}
	statsPerName := make(map[string]*StorageStats)

	if len(info.DriverStatus) == 0 {
		return statsArray, ErrStorageStatsNotAvailable
	}
	for _, entry := range info.DriverStatus {
		key := entry[0]
		valueString := entry[1]
		fields := strings.Fields(key)
		if len(fields) != 3 || strings.ToLower(fields[1]) != "space" {
			log.Debugf("ignoring invalid storage stat: %s", key)
			continue
		}
		valueInt, err := parseDiskQuantity(valueString)
		if err != nil {
			log.Debugf("ignoring invalid value %s for stat %s: %s", valueString, key, err)
			continue
		}
		storageType := strings.ToLower(fields[0])
		stats, found := statsPerName[storageType]
		if !found {
			stats = &StorageStats{
				Name: storageType,
			}
			statsPerName[storageType] = stats
			statsArray = append(statsArray, stats)
		}

		switch strings.ToLower(fields[2]) {
		case "available":
			stats.Free = &valueInt
		case "used":
			stats.Used = &valueInt
		case "total":
			stats.Total = &valueInt
		}
	}

	return statsArray, nil
}

// parseDiskQuantity parses a string from docker into a bytes quantity,
func parseDiskQuantity(text string) (uint64, error) {
	match := diskBytesRe.FindStringSubmatch(text)
	if match == nil {
		return 0, fmt.Errorf("parsing error: invalid format")
	}
	multi, found := diskUnits[strings.ToLower(match[2])]
	if !found {
		return 0, fmt.Errorf("parsing error: unknown unit %s", match[2])
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parsing error: %s", err)
	}

	return uint64(value * float64(multi)), nil
}
