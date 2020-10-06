package nvidia

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"regexp"
	"strconv"
)

const (
	// Indices of the matched fields by the temperature regex
	tempZone  = 1
	tempValue = 2
)

type temperatureMetricsSender struct {
	regex *regexp.Regexp
}

func (temperatureMetricsSender *temperatureMetricsSender) Init() error {
	// Group 1. -> Zone name
	// Group 2. -> Temperature
	regex, err := regexp.Compile(`(\w+)@(\d+(?:[.]\d+)?)C`)
	if err != nil {
		return err
	}
	temperatureMetricsSender.regex = regex

	return nil
}

func (temperatureMetricsSender *temperatureMetricsSender) SendMetrics(sender aggregator.Sender, field string) error {
	temperatureFields := temperatureMetricsSender.regex.FindAllStringSubmatch(field, -1)
	if len(temperatureFields) <= 0 {
		return errors.New("could not parse temperature fields")
	}

	for i := 0; i < len(temperatureFields); i++ {
		tempValue, err := strconv.ParseFloat(temperatureFields[i][tempValue], 64)
		if err != nil {
			return err
		}
		temperatureZoneTags := []string{fmt.Sprintf("zone:%s", temperatureFields[i][tempZone])}
		sender.Gauge("nvidia.jetson.gpu.temp", tempValue, "", temperatureZoneTags)
	}

	return nil
}
