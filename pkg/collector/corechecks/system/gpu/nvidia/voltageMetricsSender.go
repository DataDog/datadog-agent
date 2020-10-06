package nvidia

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"regexp"
	"strconv"
)

const (
	// Indices of the matched fields by the voltage regex
	voltageProbeName = 1
	currentVoltage   = 2
	averageVoltage   = 3
)

type voltageMetricsSender struct {
	regex *regexp.Regexp
}

func (voltageMetricsSender *voltageMetricsSender) Init() error {
	// Group 1. -> Voltage probe name
	// Group 2. -> Current voltage
	// Group 3. -> Average voltage
	regex, err := regexp.Compile(`(\w+)\s+(\d+)/(\d+)(?:\s+|$)`)
	if err != nil {
		return err
	}
	voltageMetricsSender.regex = regex

	return nil
}

func (voltageMetricsSender *voltageMetricsSender) SendMetrics(sender aggregator.Sender, field string) error {
	voltageFields := voltageMetricsSender.regex.FindAllStringSubmatch(field, -1)
	if len(voltageFields) <= 0 {
		return errors.New("could not parse voltage fields")
	}

	for i := 0; i < len(voltageFields); i++ {
		voltageProbeTags := []string{fmt.Sprintf("probe:%s", voltageFields[i][voltageProbeName])}
		instantVoltage, err := strconv.ParseFloat(voltageFields[i][currentVoltage], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.gpu.vdd.instant", instantVoltage, "", voltageProbeTags)

		averageVoltage, err := strconv.ParseFloat(voltageFields[i][averageVoltage], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.gpu.vdd.average", averageVoltage, "", voltageProbeTags)
	}

	return nil
}
