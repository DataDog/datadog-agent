// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package nvidia

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"regexp"
	"strconv"
)

type temperatureMetricsSender struct {
	regex *regexp.Regexp
}

func (temperatureMetricsSender *temperatureMetricsSender) Init() error {
	regex, err := regexp.Compile(`(?P<tempZone>\w+)@(?P<tempValue>\d+(?:[.]\d+)?)C`)
	if err != nil {
		return err
	}
	temperatureMetricsSender.regex = regex

	return nil
}

func (temperatureMetricsSender *temperatureMetricsSender) SendMetrics(sender aggregator.Sender, field string) error {
	r := temperatureMetricsSender.regex
	temperatureFields := r.FindAllStringSubmatch(field, -1)
	if len(temperatureFields) <= 0 {
		return errors.New("could not parse temperature fields")
	}

	for i := 0; i < len(temperatureFields); i++ {
		tempValue, err := strconv.ParseFloat(temperatureFields[i][regexSubexpIndex(r, "tempValue")], 64)
		if err != nil {
			return err
		}
		temperatureZoneTags := []string{fmt.Sprintf("zone:%s", temperatureFields[i][regexSubexpIndex(r, "tempZone")])}
		sender.Gauge("nvidia.jetson.gpu.temp", tempValue, "", temperatureZoneTags)
	}

	return nil
}
