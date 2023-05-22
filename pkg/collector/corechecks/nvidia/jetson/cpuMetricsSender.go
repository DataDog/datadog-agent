// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jetson

package nvidia

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

type cpuMetricSender struct {
	cpusRegex     *regexp.Regexp
	cpuUsageRegex *regexp.Regexp
}

func (cpuMetricSender *cpuMetricSender) Init() error {
	// List of CPUs and their usage/frequency, e.g. 2%@102,1%@102,0%@102,0%@102,off,off,off,off
	regex, err := regexp.Compile(`CPU\s*\[((?:.,?)+)]`)
	if err != nil {
		return err
	}
	cpuMetricSender.cpusRegex = regex

	regex, err = regexp.Compile(`(?P<cpuUsage>\d+)%@(?P<cpuFreq>\d+)|off`)
	if err != nil {
		return err
	}
	cpuMetricSender.cpuUsageRegex = regex

	return nil
}

func (cpuMetricSender *cpuMetricSender) SendMetrics(sender aggregator.Sender, field string) error {
	cpuFields := cpuMetricSender.cpusRegex.FindAllStringSubmatch(field, -1)
	if len(cpuFields) <= 0 {
		return errors.New("could not parse CPU usage fields")
	}
	cpus := strings.Split(cpuFields[0][1], ",")
	inactiveCpus := 0
	for i := 0; i < len(cpus); i++ {
		cpuTags := []string{fmt.Sprintf("cpu:%d", i)}
		cpuAndFreqFields := regexFindStringSubmatchMap(cpuMetricSender.cpuUsageRegex, cpus[i])
		if cpuAndFreqFields == nil {
			// No match
			return fmt.Errorf("could not parse CPU usage field of CPU %d", i)
		} else if cpuAndFreqFields[""] == "off" {
			sender.Gauge("nvidia.jetson.cpu.usage", 0.0, "", cpuTags)
			sender.Gauge("nvidia.jetson.cpu.freq", 0.0, "", cpuTags)
			inactiveCpus++
		} else {
			cpuUsage, err := strconv.ParseFloat(cpuAndFreqFields["cpuUsage"], 64)
			if err != nil {
				return err
			}
			sender.Gauge("nvidia.jetson.cpu.usage", cpuUsage, "", cpuTags)
			cpuFreq, err := strconv.ParseFloat(cpuAndFreqFields["cpuFreq"], 64)
			if err != nil {
				return err
			}
			sender.Gauge("nvidia.jetson.cpu.freq", cpuFreq, "", cpuTags)
		}
	}

	sender.Gauge("nvidia.jetson.cpu.inactive_count", float64(inactiveCpus), "", nil)
	sender.Gauge("nvidia.jetson.cpu.total_count", float64(len(cpus)), "", nil)

	return nil
}
