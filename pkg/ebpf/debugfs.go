// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package ebpf

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var myPid int

func init() {
	myPid = manager.Getpid()
}

// KprobeStats is the count of hits and misses for a kprobe/kretprobe
type KprobeStats struct {
	Hits   int64
	Misses int64
}

// event name format is p|r_<funcname>_<uid>_<pid>
var eventRegexp = regexp.MustCompile(`^((?:p|r)_.+?)_([^_]*)_([^_]*)$`)

// GetProbeStats gathers stats about the # of kprobes triggered /missed by reading the kprobe_profile file
func GetProbeStats() map[string]int64 {
	root, err := tracefs.Root()
	if err != nil {
		log.Debugf("error getting tracefs root path: %s", err)
		return map[string]int64{}
	}

	return getProbeStats(0, filepath.Join(root, "kprobe_profile"))
}

func getProbeStats(pid int, profile string) map[string]int64 {
	if pid == 0 {
		pid = myPid
	}

	m, err := readKprobeProfile(profile)
	if err != nil {
		log.Debugf("error retrieving probe stats: %s", err)
		return map[string]int64{}
	}

	res := make(map[string]int64, 2*len(m))
	for event, st := range m {
		parts := eventRegexp.FindStringSubmatch(event)
		if len(parts) > 2 {
			// only get stats for our pid
			if len(parts) > 3 {
				parsePid, err := strconv.ParseInt(parts[3], 10, 32)
				if err != nil || int(parsePid) != pid {
					continue
				}
			}
			// strip UID and PID from name
			event = parts[1]
		}
		event = strings.ToLower(event)
		res[fmt.Sprintf("%s_hits", event)] = st.Hits
		res[fmt.Sprintf("%s_misses", event)] = st.Misses
	}

	return res
}

// GetProbeTotals returns the total number of kprobes triggered or missed by reading the kprobe_profile file
func GetProbeTotals() KprobeStats {
	stats := KprobeStats{}
	root, err := tracefs.Root()
	if err != nil {
		log.Debugf("error getting tracefs root path: %s", err)
		return stats
	}

	m, err := readKprobeProfile(filepath.Join(root, "kprobe_profile"))
	if err != nil {
		log.Debugf("error retrieving probe stats: %s", err)
		return stats
	}

	for _, st := range m {
		stats.Hits += st.Hits
		stats.Misses += st.Misses
	}
	return stats
}

// readKprobeProfile reads a /sys/kernel/[debug/]tracing/kprobe_profile file and returns a map of probe -> stats
func readKprobeProfile(path string) (map[string]KprobeStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening kprobe profile file at: %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	stats := map[string]KprobeStats{}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue
		}

		hits, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			log.Debugf("error parsing kprobe_profile output for hits (%s): %s", fields[1], err)
			continue
		}

		misses, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			log.Debugf("error parsing kprobe_profile output for miss (%s): %s", fields[2], err)
			continue
		}

		stats[fields[0]] = KprobeStats{
			Hits:   hits,
			Misses: misses,
		}
	}

	return stats, nil
}
