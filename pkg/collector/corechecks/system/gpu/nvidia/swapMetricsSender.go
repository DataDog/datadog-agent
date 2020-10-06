package nvidia

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"regexp"
	"strconv"
)

const (
	// Indices of the matched fields by the Swap/Cache regex
	swapUsed  = 1
	totalSwap = 2
	swapUnit  = 3
	cached    = 4
	cacheUnit = 5
)

type swapMetricsSender struct {
	regex *regexp.Regexp
}

func (swapMetricsSender *swapMetricsSender) Init() error {
	// Group 1. -> Used
	// Group 2. -> Total
	// Group 3. -> Unit
	// Group 4. -> Cached
	// Group 5. -> Unit
	regex, err := regexp.Compile(`SWAP\s*(\d+)/(\d+)([kKmMgG][bB])\s*\(cached\s*(\d+)([kKmMgG][bB])\)`)
	if err != nil {
		return err
	}
	swapMetricsSender.regex = regex

	return nil
}

func (swapMetricsSender *swapMetricsSender) SendMetrics(sender aggregator.Sender, field string) error {
	swapFields := swapMetricsSender.regex.FindAllStringSubmatch(field, -1)
	if len(swapFields) != 1 {
		// SWAP is not present on all devices
		return nil
	}

	swapMultiplier := getSizeMultiplier(swapFields[0][swapUnit])

	swapUsed, err := strconv.ParseFloat(swapFields[0][swapUsed], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.swap.used", swapUsed*swapMultiplier, "", nil)

	totalSwap, err := strconv.ParseFloat(swapFields[0][totalSwap], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.swap.total", totalSwap*swapMultiplier, "", nil)

	cacheMultiplier := getSizeMultiplier(swapFields[0][cacheUnit])
	cached, err := strconv.ParseFloat(swapFields[0][cached], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.swap.cached", cached*cacheMultiplier, "", nil)

	return nil
}
