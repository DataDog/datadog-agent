package nvidia

import (
	"errors"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"regexp"
	"strconv"
)

const (
	// Indices of the matched fields by the RAM regex
	ramUsed          = 1
	totalRAM         = 2
	ramUnit          = 3
	numFreeBlock     = 4
	largestFreeBlock = 5
	lfbUnit          = 6
)

type ramMetricSender struct {
	regex *regexp.Regexp
}

func (ramMetricSender *ramMetricSender) Init() error {
	// Group 1. -> Used
	// Group 2. -> Total
	// Group 3. -> Unit
	// Group 4. -> Number of LFB
	// Group 5. -> LFB
	// Group 6. -> Unit
	regex, err := regexp.Compile(`RAM\s*(\d+)/(\d+)([kKmMgG][bB])\s*\(lfb\s*(\d+)x(\d+)([kKmMgG][bB])\)`)
	if err != nil {
		return err
	}
	ramMetricSender.regex = regex
	return nil
}

func (ramMetricSender *ramMetricSender) SendMetrics(sender aggregator.Sender, field string) error {
	ramFields := ramMetricSender.regex.FindAllStringSubmatch(field, -1)
	if len(ramFields) != 1 {
		return errors.New("could not parse RAM fields")
	}

	ramMultiplier := getSizeMultiplier(ramFields[0][ramUnit])

	usedRAM, err := strconv.ParseFloat(ramFields[0][ramUsed], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.mem.used", usedRAM*ramMultiplier, "", nil)

	totalRAM, err := strconv.ParseFloat(ramFields[0][totalRAM], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.mem.total", totalRAM*ramMultiplier, "", nil)

	// lfb NxXMB, X is the largest free block. N is the number of free blocks of this size.
	lfbMultiplier := getSizeMultiplier(ramFields[0][lfbUnit])

	largestFreeBlock, err := strconv.ParseFloat(ramFields[0][largestFreeBlock], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.mem.lfb", largestFreeBlock*lfbMultiplier, "", nil)

	numFreeBlocks, err := strconv.ParseFloat(ramFields[0][numFreeBlock], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.mem.n_lfb", numFreeBlocks, "", nil)

	return nil
}
