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

type gpuMetricSender struct {
	regex *regexp.Regexp
}

func (gpuMetricSender *gpuMetricSender) Init() error {
	regex, err := regexp.Compile(`EMC_FREQ\s*(?P<emcPct>\d+)%(?:@(?P<emcFreq>\d+))?\s*GR3D_FREQ\s*(?P<gpuPct>\d+)%(?:@(?P<gpuFreq>\d+))?`)
	if err != nil {
		return err
	}
	gpuMetricSender.regex = regex

	return nil
}

func (gpuMetricSender *gpuMetricSender) SendMetrics(sender aggregator.Sender, field string) error {
	gpuFields := regexFindStringSubmatchMap(gpuMetricSender.regex, field)
	if gpuFields == nil {
		return errors.New("could not parse GPU usage fields")
	}

	emcPct, err := strconv.ParseFloat(gpuFields["emcPct"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.emc.usage", emcPct, "", nil)

	if len(gpuFields["emcFreq"]) > 0 {
		emcFreq, err := strconv.ParseFloat(gpuFields["emcFreq"], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.emc.freq", emcFreq, "", nil)
	}

	gpuPct, err := strconv.ParseFloat(gpuFields["gpuPct"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.usage", gpuPct, "", nil)

	if len(gpuFields["gpuFreq"]) > 0 {
		gpuFreq, err := strconv.ParseFloat(gpuFields["gpuFreq"], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.gpu.freq", gpuFreq, "", nil)
	}

	return nil
}
