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

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

type voltageMetricsSender struct {
	regex *regexp.Regexp
}

func (voltageMetricsSender *voltageMetricsSender) Init() error {
	regex, err := regexp.Compile(`(?P<voltageProbeName>\w+)\s+(?P<currentVoltage>\d+)/(?P<averageVoltage>\d+)(?:\s+|$)`)
	if err != nil {
		return err
	}
	voltageMetricsSender.regex = regex

	return nil
}

func (voltageMetricsSender *voltageMetricsSender) SendMetrics(sender aggregator.Sender, field string) error {
	r := voltageMetricsSender.regex
	voltageFields := r.FindAllStringSubmatch(field, -1)
	if len(voltageFields) <= 0 {
		return errors.New("could not parse voltage fields")
	}

	for i := 0; i < len(voltageFields); i++ {
		voltageProbeTags := []string{fmt.Sprintf("probe:%s", voltageFields[i][regexSubexpIndex(r, "voltageProbeName")])}
		instantVoltage, err := strconv.ParseFloat(voltageFields[i][regexSubexpIndex(r, "currentVoltage")], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.power.instant", instantVoltage, "", voltageProbeTags)

		averageVoltage, err := strconv.ParseFloat(voltageFields[i][regexSubexpIndex(r, "averageVoltage")], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.power.average", averageVoltage, "", voltageProbeTags)
	}

	return nil
}
