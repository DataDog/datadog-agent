// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package cpu

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"

	log "github.com/cihub/seelog"
)

var prefix = "" // only used for testing
var listRangeRegex = regexp.MustCompile("([0-9]+)-([0-9]+)$")

// sysCPUInt reads an integer from a file in /sys/devices/system/cpu
func sysCPUInt(path string) (uint64, bool) {
	filepath := prefix + "/sys/devices/system/cpu/" + path
	content, err := os.ReadFile(filepath)
	if err != nil {
		return 0, false
	}

	value, err := strconv.ParseUint(strings.TrimSpace(string(content)), 0, 64)
	if err != nil {
		log.Warnf("file %s did not contain a valid integer", filepath)
		return 0, false
	}

	return value, true
}

// sysCPUSize reads a value with a K/M/G suffix from a file in /sys/devices/system/cpu
func sysCPUSize(path string) (uint64, bool) {
	filepath := prefix + "/sys/devices/system/cpu/" + path
	content, err := os.ReadFile(filepath)
	if err != nil {
		return 0, false
	}

	s := strings.TrimSpace(string(content))
	if s == "" {
		log.Warnf("file %s was empty", filepath)
		return 0, false
	}

	mult := uint64(1)
	switch s[len(s)-1] {
	case 'K':
		mult = 1024
	case 'M':
		mult = 1024 * 1024
	case 'G':
		mult = 1024 * 1024 * 1024
	}
	if mult > 1 {
		s = s[:len(s)-1]
	}

	value, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		log.Warnf("file %s did not contain a valid size", filepath)
		return 0, false
	}

	return value * mult, true
}

// sysCPUList reads a list of integers, comma-seprated with ranges (`0-5,7-11`)
// from a file in /sys/devices/system/cpu.  The return value is the set of
// integers included in the list (for the example above, {0, 1, 2, 3, 4, 5, 7,
// 8, 9, 10, 11}).
func sysCPUList(path string) (map[uint64]struct{}, bool) {
	content, err := os.ReadFile(prefix + "/sys/devices/system/cpu/" + path)
	if err != nil {
		return nil, false
	}

	result := map[uint64]struct{}{}
	contentStr := strings.TrimSpace(string(content))
	if contentStr == "" {
		return result, true
	}

	for _, elt := range strings.Split(contentStr, ",") {
		if submatches := listRangeRegex.FindStringSubmatch(elt); submatches != nil {
			// Handle the NN-NN form, inserting each included integer into the set
			first, err := strconv.ParseUint(submatches[1], 0, 64)
			if err != nil {
				return nil, false
			}
			last, err := strconv.ParseUint(submatches[2], 0, 64)
			if err != nil {
				return nil, false
			}
			for i := first; i <= last; i++ {
				result[i] = struct{}{}
			}
		} else {
			// Handle a simple integer, just inserting it into the set
			i, err := strconv.ParseUint(elt, 0, 64)
			if err != nil {
				return nil, false
			}
			result[i] = struct{}{}
		}
	}

	return result, true
}

// readProcCPUInfo reads /proc/cpuinfo.  The file is structured as a set of
// blank-line-separated stanzas, and each stanza is a map of string to string,
// with whitespace stripped.
func readProcCPUInfo() ([]map[string]string, error) {
	file, err := os.Open(prefix + "/proc/cpuinfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var stanzas []map[string]string
	var stanza map[string]string

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			stanza = nil
			continue
		}

		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if stanza == nil {
			stanza = make(map[string]string)
			stanzas = append(stanzas, stanza)
		}
		stanza[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	if scanner.Err() != nil {
		err = scanner.Err()
		return nil, err
	}

	// On some platforms, such as rPi, there are stanzas in this file that do
	// not correspond to processors.  It doesn't seem this file is intended for
	// machine consumption!  So, we filter those out.
	var results []map[string]string
	for _, stanza := range stanzas {
		if _, found := stanza["processor"]; !found {
			continue
		}
		results = append(results, stanza)
	}

	return results, nil
}
