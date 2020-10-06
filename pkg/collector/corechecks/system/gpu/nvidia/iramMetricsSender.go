package nvidia

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"regexp"
	"strconv"
)

const (
	// Indices of the matched fields by the Icache regex
	iramUsed    = 1
	iramTotal   = 2
	iramUnit    = 3
	iramLfb     = 4
	iramLfbUnit = 5
)

type iramMetricSender struct {
	regex *regexp.Regexp
}

func (iramMetricSender *iramMetricSender) Init() error {
	// Group 1. -> Used
	// Group 2. -> Total
	// Group 3. -> Unit
	// Group 4. -> LFB
	// Group 5. -> Unit
	regex, err := regexp.Compile(`IRAM\s*(\d+)/(\d+)([kKmMgG][bB])\s*\(lfb\s*(\d+)([kKmMgG][bB])\)`)
	if err != nil {
		return err
	}
	iramMetricSender.regex = regex
	return nil
}

func (iramMetricSender *iramMetricSender) SendMetrics(sender aggregator.Sender, field string) error {
	iramFields := iramMetricSender.regex.FindAllStringSubmatch(field, -1)
	if len(iramFields) != 1 {
		// IRAM is not present on all devices
		return nil
	}

	iramMultiplier := getSizeMultiplier(iramFields[0][iramUnit])

	usedIRAM, err := strconv.ParseFloat(iramFields[0][iramUsed], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.iram.used", usedIRAM*iramMultiplier, "", nil)

	totalIRAM, err := strconv.ParseFloat(iramFields[0][iramTotal], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.iram.total", totalIRAM*iramMultiplier, "", nil)

	iramLfbMultiplier := getSizeMultiplier(iramFields[0][iramLfbUnit])
	iramLfb, err := strconv.ParseFloat(iramFields[0][iramLfb], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.iram.lfb", iramLfb*iramLfbMultiplier, "", nil)

	return nil
}
