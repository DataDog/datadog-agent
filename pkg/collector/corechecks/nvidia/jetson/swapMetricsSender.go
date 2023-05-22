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

type swapMetricsSender struct {
	regex *regexp.Regexp
}

func (swapMetricsSender *swapMetricsSender) Init() error {
	regex, err := regexp.Compile(`SWAP\s*(?P<usedSwap>\d+)/(?P<totalSwap>\d+)(?P<swapUnit>[kKmMgG][bB])\s*\(cached\s*(?P<cached>\d+)(?P<cachedUnit>[kKmMgG][bB])\)`)
	if err != nil {
		return err
	}
	swapMetricsSender.regex = regex

	return nil
}

func (swapMetricsSender *swapMetricsSender) SendMetrics(sender aggregator.Sender, field string) error {
	swapFields := regexFindStringSubmatchMap(swapMetricsSender.regex, field)
	if swapFields == nil {
		// SWAP is not present on all devices
		return nil
	}

	swapMultiplier := getSizeMultiplier(swapFields["swapUnit"])

	swapUsed, err := strconv.ParseFloat(swapFields["usedSwap"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.swap.used", swapUsed*swapMultiplier, "", nil)

	totalSwap, err := strconv.ParseFloat(swapFields["totalSwap"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.swap.total", totalSwap*swapMultiplier, "", nil)

	cacheMultiplier := getSizeMultiplier(swapFields["cachedUnit"])
	cached, err := strconv.ParseFloat(swapFields["cached"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.swap.cached", cached*cacheMultiplier, "", nil)

	return nil
}
