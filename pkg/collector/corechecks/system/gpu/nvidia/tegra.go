// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package nvidia

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const (
	tegraCheckName       = "tegra"
	defaultRetryDuration = 5 * time.Second
	defaultRetries       = 3

	kb = 1024
	mb = kb * 1024
	gb = mb * 1024

	// Indices of the regex in the 'regexes' variable below
	regexRAMIdx       = 0
	regexSwapCacheIdx = 1
	regexIRamIdx      = 2

	// Regex used to parse the GPU usage and frequency => e.g. EMC_FREQ 7%@408 GR3D_FREQ 0%@76
	regexGpuUsageIdx = 3

	// Regex used to parse the CPU usage section => e.g. CPU [2%@102,1%@102,0%@102,0%@102]
	regexCpuUsageIdx = 4

	// Regex used to parse the temperature information => e.g. thermal@41C
	regexTemperatureIdx = 5

	// Regex used to parse the voltage information => e.g. POM_5V_IN 900/943
	regexVoltageIdx = 6

	// Regex used to parse cpu and freq => e.g. 2%@102
	regexCpuFreqIdx = 7

	// Indices of the matched fields by the RAM regex
	ramUsed          = 1
	totalRam         = 2
	ramUnit          = 3
	numFreeBlock     = 4
	largestFreeBlock = 5
	lfbUnit          = 6

	// Indices of the matched fields by the Swap/Cache regex
	swapUsed  = 1
	totalSwap = 2
	swapUnit  = 3
	cached    = 4
	cacheUnit = 5

	// Indices of the matched fields by the Icache regex

	// Indices of the matched fields by the GPU usage regex
	emcPct  = 1
	emcFreq = 2
	gpuPct  = 3
	gpuFreq = 4

	voltageProbeName = 1
	currentVoltage   = 2
	averageVoltage   = 3

	tempZone  = 1
	tempValue = 2

	cpuUsage = 1
	cpuFreq  = 2
)

var regexes = [...]string{
	// Group 1.	-> Used
	// Group 2.	-> Total
	// Group 3.	-> Unit
	// Group 4.	-> Number of LFB
	// Group 5.	-> LFB
	// Group 6.	-> Unit
	`RAM\s*(\d+)/(\d+)([kKmMgG][bB])\s*\(lfb\s*(\d+)x(\d+)([kKmMgG][bB])\)`,

	// Group 1.	-> Used
	// Group 2.	-> Total
	// Group 3.	-> Unit
	// Group 4. -> Cached
	// Group 5. -> Unit
	`SWAP\s*(\d+)\/(\d+)([kKmMgG][bB])\s*\(cached\s*(\d+)([kKmMgG][bB])\)`,

	// Group 1.	-> Used
	// Group 2.	-> Total
	// Group 3.	-> Unit
	// Group 4.	-> LFB
	// Group 5.	-> Unit
	`IRAM\s*(\d+)\/(\d+)([kKmMgG][bB])\s*\(lfb\s*(\d+)([kKmMgG][bB])\)`,

	// Group 1.	-> EMC %
	// Group 2.	-> EMC Freq (opt)
	// Group 3.	-> GPU %
	// Group 4.	-> GPU Freq (opt)
	`EMC_FREQ\s*(\d+)%(?:@(\d+))?\s*GR3D_FREQ\s*(\d+)%(?:@(\d+))?`,

	`CPU\s*\[((?:\d+%@\d+,?)+)\]`,

	// Group 1.	-> Zone name
	// Group 2.	-> Temperature
	`(\w+)@(\d+(?:[.]\d+)?)C`,

	// Group 1.	-> Voltage probe name
	// Group 2.	-> Current voltage
	// Group 2.	-> Average voltage
	`(\w+)\s+(\d+)\/(\d+)(?:\s+|$)`,

	// Group 1. -> CPU usage
	// Group 2. -> CPU freq
	`(\d+)%@(\d+)`,
}

// retryExitError converts `exec.ExitError`s to `check.RetryableError`s, so that checks using this
// are retried.
func retryExitError(err error) error { // nolint Used only on some architectures
	switch err.(type) {
	case *exec.ExitError: // error type returned when the process exits with non-zero status
		return check.RetryableError{Err: err}
	default:
		return err
	}
}

// TegraCheck contains the field for the TegraCheck
type TegraCheck struct {
	core.CheckBase

	// Indicates that this check has been scheduled and is running.
	running uint32

	// The path to the tegrastats binary. Defaults to /usr/bin/tegrastats
	tegraStatsPath string

	// The command line options for tegrastats
	commandOpts []string

	regexes []*regexp.Regexp

	stop     chan struct{}
	stopDone chan struct{}
}

// Interval returns the scheduling time for the check.
// Returns 0 since we're a long-running check.
func (c *TegraCheck) Interval() time.Duration {
	return 0
}

func getSizeMultiplier(unit string) float64 {
	switch strings.ToLower(unit) {
	case "kb":
		return kb
	case "mb":
		return mb
	case "gb":
		return gb
	}
	return 1
}

func (c *TegraCheck) sendRamMetrics(sender aggregator.Sender, field string) error {
	ramFields := c.regexes[regexRAMIdx].FindAllStringSubmatch(field, -1)
	if len(ramFields) != 1 {
		return errors.New("could not parse RAM fields")
	}

	ramMultiplier := getSizeMultiplier(ramFields[0][ramUnit])

	usedRam, err := strconv.ParseFloat(ramFields[0][ramUsed], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.mem.used", usedRam*ramMultiplier, "", nil)

	totalRam, err := strconv.ParseFloat(ramFields[0][totalRam], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.mem.total", totalRam*ramMultiplier, "", nil)

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

func (c *TegraCheck) sendSwapMetrics(sender aggregator.Sender, field string) error {
	swapFields := c.regexes[regexSwapCacheIdx].FindAllStringSubmatch(field, -1)
	if len(swapFields) != 1 {
		return errors.New("could not parse SWAP fields")
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

func (c *TegraCheck) sendGpuUsageMetrics(sender aggregator.Sender, field string) error {
	gpuFields := c.regexes[regexGpuUsageIdx].FindAllStringSubmatch(field, -1)
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

func (c *TegraCheck) sendCpuUsageMetrics(sender aggregator.Sender, field string) error {
	cpuFields := c.regexes[regexCpuUsageIdx].FindAllStringSubmatch(field, -1)
	if len(cpuFields) <= 0 {
		return errors.New("could not parse CPU usage fields")
	}
	cpus := strings.Split(cpuFields[0][1], ",")

	for i := 0; i < len(cpus); i++ {
		cpuAndFreqFields := c.regexes[regexCpuFreqIdx].FindAllStringSubmatch(cpus[i], -1)
		cpuUsage, err := strconv.ParseFloat(cpuAndFreqFields[0][cpuUsage], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.cpu.usage", cpuUsage, "", []string{strconv.Itoa(i)})

		cpuFreq, err := strconv.ParseFloat(cpuAndFreqFields[0][cpuFreq], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.cpu.freq", cpuFreq, "", []string{strconv.Itoa(i)})
	}

	return nil
}

func (c *TegraCheck) sendTemperatureMetrics(sender aggregator.Sender, field string) error {
	temperatureFields := c.regexes[regexTemperatureIdx].FindAllStringSubmatch(field, -1)
	if len(temperatureFields) <= 0 {
		return errors.New("could not parse temperature fields")
	}

	for i := 0; i < len(temperatureFields); i++ {
		tempValue, err := strconv.ParseFloat(temperatureFields[i][tempValue], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.gpu.temp", tempValue, "", []string{temperatureFields[i][tempZone]})
	}

	return nil
}

func (c *TegraCheck) sendVoltageMetrics(sender aggregator.Sender, field string) error {
	voltageFields := c.regexes[regexVoltageIdx].FindAllStringSubmatch(field, -1)
	if len(voltageFields) <= 0 {
		return errors.New("could not parse voltage fields")
	}

	for i := 0; i < len(voltageFields); i++ {
		voltageProbeName := voltageFields[i][voltageProbeName]

		currentVoltage, err := strconv.ParseFloat(voltageFields[i][currentVoltage], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.gpu.vdd.current", currentVoltage, "", []string{voltageProbeName})

		averageVoltage, err := strconv.ParseFloat(voltageFields[i][averageVoltage], 64)
		if err != nil {
			return err
		}
		sender.Gauge("nvidia.jetson.gpu.vdd.average", averageVoltage, "", []string{voltageProbeName})
	}

	return nil
}

// Run executes the check
func (c *TegraCheck) Run() error {
	atomic.StoreUint32(&c.running, 1)
	err := check.Retry(defaultRetryDuration, defaultRetries, c.run, c.String())
	atomic.StoreUint32(&c.running, 0)

	return err
}

func (c *TegraCheck) processTegraStatsOutput(tegraStatsOuptut string) error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	err = c.sendRamMetrics(sender, tegraStatsOuptut)
	if err != nil {
		return nil
	}
	err = c.sendSwapMetrics(sender, tegraStatsOuptut)
	if err != nil {
		return nil
	}
	err = c.sendGpuUsageMetrics(sender, tegraStatsOuptut)
	if err != nil {
		return nil
	}
	err = c.sendCpuUsageMetrics(sender, tegraStatsOuptut)
	if err != nil {
		return nil
	}
	err = c.sendTemperatureMetrics(sender, tegraStatsOuptut)
	if err != nil {
		return nil
	}
	err = c.sendVoltageMetrics(sender, tegraStatsOuptut)
	if err != nil {
		return nil
	}
	sender.Commit()
	return nil
}

func (c *TegraCheck) run() error {
	select {
	// poll the stop channel once to make sure no stop was requested since the last call to `run`
	case <-c.stop:
		log.Info("Not starting %s check: stop requested", tegraCheckName)
		c.stopDone <- struct{}{}
		return nil
	default:
	}

	cmd := exec.Command(c.tegraStatsPath, c.commandOpts...)

	// Parse the standard output for the stats
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			if err = c.processTegraStatsOutput(in.Text()); err != nil {
				_ = log.Error(err)
			}
		}
	}()

	// forward the standard error to the Agent logger
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			_ = log.Error(in.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
		return retryExitError(err)
	}

	processDone := make(chan error)
	go func() {
		processDone <- cmd.Wait()
	}()

	select {
	case err = <-processDone:
		return retryExitError(err)
	case <-c.stop:
		err = cmd.Process.Signal(os.Kill)
		if err != nil {
			_ = log.Errorf("unable to stop %s check: %s", tegraCheckName, err)
		}
	}

	err = <-processDone
	c.stopDone <- struct{}{}
	return err
}

// Configure the GPU check
func (c *TegraCheck) Configure(data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(data, source)
	if err != nil {
		return err
	}

	// TODO: Make this configurable
	c.tegraStatsPath = "/usr/bin/tegrastats"

	// Since our interval is 0 because we're a long running check, we can use the CheckBase.Interval() as
	// the tegrastats reporting interval
	c.commandOpts = []string{
		fmt.Sprintf("--interval %d", int64(c.CheckBase.Interval()*time.Millisecond)),
	}

	c.regexes = make([]*regexp.Regexp, len(regexes))
	for idx, regex := range regexes {
		c.regexes[idx] = regexp.MustCompile(regex)
	}

	return nil
}

func tegraCheckFactory() check.Check {
	return &TegraCheck{
		CheckBase: core.NewCheckBase(tegraCheckName),
	}
}

func init() {
	core.RegisterCheck(tegraCheckName, tegraCheckFactory)
}
