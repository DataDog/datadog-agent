// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"

	"github.com/alecthomas/units"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MemoryMonitor monitors cgroups' memory usage
type MemoryMonitor = cgroups.MemoryController

const maxProfileCount = 10

func getActionCallback(action string) (func(), string, error) {
	switch action {
	case "gc":
		return runtime.GC, "garbage collector", nil
	case "log":
		return func() {}, "nothing", nil
	case "profile":
		return func() {
			tmpDir := os.TempDir()
			tmpFiles, err := os.ReadDir(tmpDir)
			if err != nil {
				log.Errorf("Failed to list old memory profiles: %s", err)
			} else {
				var oldProfiles []os.FileInfo
				for _, tmpFile := range tmpFiles {
					if strings.HasPrefix(tmpFile.Name(), "memcg-pprof-heap") {
						tmpFileInfo, err := tmpFile.Info()
						if err != nil {
							log.Errorf("Failed to get file info for: %s", tmpFile.Name())
							continue
						}
						oldProfiles = append(oldProfiles, tmpFileInfo)
					}
				}

				sort.Slice(oldProfiles, func(i, j int) bool {
					return oldProfiles[i].ModTime().After(oldProfiles[j].ModTime())
				})

				for i := len(oldProfiles) - 1; i >= 0 && i >= maxProfileCount-1; i-- {
					os.Remove(filepath.Join(tmpDir, oldProfiles[i].Name()))
					oldProfiles = oldProfiles[:i]
				}
			}

			memProfile, err := os.CreateTemp(tmpDir, "memcg-pprof-heap")
			if err != nil {
				log.Errorf("Failed to generate memory profile: %s", err)
				return
			}

			defer func() {
				if err := memProfile.Close(); err != nil {
					log.Errorf("Failed to generate memory profile: %s", err)
				}
			}()

			if err := pprof.WriteHeapProfile(memProfile); err != nil {
				log.Errorf("Failed to generate memory profile: %s", err)
				return
			}

			log.Infof("Wrote memory profile to %s", memProfile.Name())
		}, "heap profile", nil
	default:
		return nil, "", fmt.Errorf("unknown memory controller action '%s'", action)
	}
}

// NewMemoryMonitor instantiates a new memory monitor
func NewMemoryMonitor(kind string, containerized bool, pressureLevels map[string]string, thresholds map[string]string) (*MemoryMonitor, error) {
	memoryMonitors := make([]cgroups.MemoryMonitor, 0, len(pressureLevels)+len(thresholds))

	for pressureLevel, action := range pressureLevels {
		actionCallback, name, err := getActionCallback(action)
		if err != nil {
			return nil, err
		}

		log.Infof("New memory pressure monitor on level %s with action %s", pressureLevel, name)
		memoryMonitors = append(memoryMonitors, cgroups.MemoryPressureMonitor(func() {
			log.Infof("Memory pressure reached level '%s', triggering %s", pressureLevel, name)
			actionCallback()
		}, pressureLevel))
	}

	for threshold, action := range thresholds {
		actionCallback, name, err := getActionCallback(action)
		if err != nil {
			return nil, err
		}

		monitorCallback := func() {
			log.Infof("Memory pressure above %s threshold, triggering %s", threshold, name)
			actionCallback()
		}

		var memoryMonitor cgroups.MemoryMonitor
		threshold = strings.TrimSpace(threshold)
		if strings.HasSuffix(threshold, "%") {
			percentage, err := strconv.Atoi(strings.TrimSuffix(threshold, "%"))
			if err != nil {
				return nil, fmt.Errorf("invalid memory threshold '%s': %w", threshold, err)
			}

			memoryMonitor = cgroups.MemoryPercentageThresholdMonitor(monitorCallback, uint64(percentage), false)
		} else {
			size, err := units.ParseBase2Bytes(strings.ToUpper(threshold))
			if err != nil {
				return nil, fmt.Errorf("invalid memory threshold '%s': %w", threshold, err)
			}

			memoryMonitor = cgroups.MemoryThresholdMonitor(monitorCallback, uint64(size), false)
		}

		log.Infof("New memory threshold monitor on level %s with action %s", threshold, name)
		memoryMonitors = append(memoryMonitors, memoryMonitor)
	}

	return cgroups.NewMemoryController(kind, containerized, memoryMonitors...)
}
