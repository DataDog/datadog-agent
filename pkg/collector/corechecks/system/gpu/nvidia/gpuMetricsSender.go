package nvidia

import (
	"errors"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"regexp"
	"strconv"
)

const (
	// Indices of the matched fields by the GPU usage regex
	emcPct  = 1
	emcFreq = 2
	gpuPct  = 3
	gpuFreq = 4
)

type gpuMetricSender struct {
	regex *regexp.Regexp
}

func (gpuMetricSender *gpuMetricSender) Init() error {
	// Group 1. -> EMC %
	// Group 2. -> EMC Freq (opt)
	// Group 3. -> GPU %
	// Group 4. -> GPU Freq (opt)
	regex, err := regexp.Compile(`EMC_FREQ\s*(\d+)%(?:@(\d+))?\s*GR3D_FREQ\s*(\d+)%(?:@(\d+))?`)
	if err != nil {
		return err
	}
	gpuMetricSender.regex = regex

	return nil
}

func (gpuMetricSender *gpuMetricSender) SendMetrics(sender aggregator.Sender, field string) error {
	gpuFields := gpuMetricSender.regex.FindAllStringSubmatch(field, -1)
	if len(gpuFields) != 1 {
		return errors.New("could not parse GPU usage fields")
	}

	emcPct, err := strconv.ParseFloat(gpuFields[0][emcPct], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.emc.usage", emcPct, "", nil)

	if len(gpuFields[0][emcFreq]) > 0 {
		emcFreq, err := strconv.ParseFloat(gpuFields[0][emcFreq], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.gpu.emc.freq", emcFreq, "", nil)
	}

	gpuPct, err := strconv.ParseFloat(gpuFields[0][gpuPct], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.usage", gpuPct, "", nil)

	if len(gpuFields[0][gpuFreq]) > 0 {
		gpuFreq, err := strconv.ParseFloat(gpuFields[0][gpuFreq], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.gpu.freq", gpuFreq, "", nil)
	}

	return nil
}
