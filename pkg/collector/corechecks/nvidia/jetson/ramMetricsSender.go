// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jetson

package nvidia

import (
	"errors"
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

type ramMetricSender struct {
	regex *regexp.Regexp
}

func (ramMetricSender *ramMetricSender) Init() error {
	regex, err := regexp.Compile(`RAM\s*(?P<usedRam>\d+)/(?P<totalRam>\d+)(?P<ramUnit>[kKmMgG][bB])\s*\(lfb\s*(?P<numFreeBlock>\d+)x(?P<largestFreeBlock>\d+)(?P<lfbUnit>[kKmMgG][bB])\)`)
	if err != nil {
		return err
	}
	ramMetricSender.regex = regex
	return nil
}

func (ramMetricSender *ramMetricSender) SendMetrics(sender aggregator.Sender, field string) error {
	ramFields := regexFindStringSubmatchMap(ramMetricSender.regex, field)
	if ramFields == nil {
		return errors.New("could not parse RAM fields")
	}

	ramMultiplier := getSizeMultiplier(ramFields["ramUnit"])

	usedRAM, err := strconv.ParseFloat(ramFields["usedRam"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.mem.used", usedRAM*ramMultiplier, "", nil)

	totalRAM, err := strconv.ParseFloat(ramFields["totalRam"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.mem.total", totalRAM*ramMultiplier, "", nil)

	// lfb NxXMB, X is the largest free block. N is the number of free blocks of this size.
	lfbMultiplier := getSizeMultiplier(ramFields["lfbUnit"])

	largestFreeBlock, err := strconv.ParseFloat(ramFields["largestFreeBlock"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.mem.lfb", largestFreeBlock*lfbMultiplier, "", nil)

	numFreeBlocks, err := strconv.ParseFloat(ramFields["numFreeBlock"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.mem.n_lfb", numFreeBlocks, "", nil)

	return nil
}
