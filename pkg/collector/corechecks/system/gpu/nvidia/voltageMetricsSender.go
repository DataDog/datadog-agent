package nvidia

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"regexp"
	"strconv"
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
		voltageProbeTags := []string{fmt.Sprintf("probe:%s", voltageFields[i][r.SubexpIndex("voltageProbeName")])}
		instantVoltage, err := strconv.ParseFloat(voltageFields[i][r.SubexpIndex("currentVoltage")], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.gpu.vdd.instant", instantVoltage, "", voltageProbeTags)

		averageVoltage, err := strconv.ParseFloat(voltageFields[i][r.SubexpIndex("averageVoltage")], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.gpu.vdd.average", averageVoltage, "", voltageProbeTags)
	}

	return nil
}
