// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jetson

package nvidia

import (
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

type iramMetricSender struct {
	regex *regexp.Regexp
}

func (iramMetricSender *iramMetricSender) Init() error {
	regex, err := regexp.Compile(`IRAM\s*(?P<iramUsed>\d+)/(?P<iramTotal>\d+)(?P<iramUnit>[kKmMgG][bB])\s*\(lfb\s*(?P<iramLfb>\d+)(?P<iramLfbUnit>[kKmMgG][bB])\)`)
	if err != nil {
		return err
	}
	iramMetricSender.regex = regex
	return nil
}

func (iramMetricSender *iramMetricSender) SendMetrics(sender aggregator.Sender, field string) error {
	iramFields := regexFindStringSubmatchMap(iramMetricSender.regex, field)
	if iramFields == nil {
		// IRAM is not present on all devices
		return nil
	}

	iramMultiplier := getSizeMultiplier(iramFields["iramUnit"])

	usedIRAM, err := strconv.ParseFloat(iramFields["iramUsed"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.iram.used", usedIRAM*iramMultiplier, "", nil)

	totalIRAM, err := strconv.ParseFloat(iramFields["iramTotal"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.iram.total", totalIRAM*iramMultiplier, "", nil)

	iramLfbMultiplier := getSizeMultiplier(iramFields["iramLfbUnit"])
	iramLfb, err := strconv.ParseFloat(iramFields["iramLfb"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.iram.lfb", iramLfb*iramLfbMultiplier, "", nil)

	return nil
}
