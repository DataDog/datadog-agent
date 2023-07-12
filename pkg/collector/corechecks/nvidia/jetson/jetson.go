// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jetson

package nvidia

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	checkName = "jetson"

	// The interval to run tegrastats at, in seconds
	tegraStatsInterval = 500 * time.Millisecond

	kb = 1024
	mb = kb * 1024
	gb = mb * 1024
)

// The configuration for the jetson check
type checkCfg struct {
	TegraStatsPath string `yaml:"tegrastats_path,omitempty"`
	UseSudo        bool   `yaml:"use_sudo,omitempty"`
}

// JetsonCheck contains the field for the JetsonCheck
type JetsonCheck struct {
	core.CheckBase
	metrics.Gauge
	// The path to the tegrastats binary. Defaults to /usr/bin/tegrastats
	tegraStatsPath string

	// The command line options for tegrastats
	commandOpts []string

	metricsSenders []metricsSender
	useSudo        bool
}

// regexFindStringSubmatchMap returns a map of strings where the keys are the name
// of the submatch groups defined in its expressions and the values are the matches,
// if any, of that group.
// A return value of nil indicates no match.
// The map will contain an empty key that holds the full match, if any. E.g.
// map[""] = fullMatch
func regexFindStringSubmatchMap(regex *regexp.Regexp, str string) map[string]string {
	matches := regex.FindStringSubmatch(str)
	if matches == nil {
		return nil
	}
	result := make(map[string]string)
	names := regex.SubexpNames()
	for i, match := range matches {
		result[names[i]] = match
	}
	return result
}

// Only available in Go 15, so implement it here for now
func regexSubexpIndex(regex *regexp.Regexp, name string) int {
	if name != "" {
		for i, s := range regex.SubexpNames() {
			if name == s {
				return i
			}
		}
	}
	return -1
}

// getSizeMultiplier returns a multiplier for a given unit, i.e. kb = 1024, mb = 1024*1024 etc...
// If the unit is not one of "kb", "mb" or "gb", it returns 1
func getSizeMultiplier(unit string) float64 {
	switch strings.ToLower(unit) {
	case "kb":
		return kb
	case "mb":
		return mb
	case "gb":
		return gb
	default:
		return 1
	}
}

// Parses the output of tegrastats
func (c *JetsonCheck) processTegraStatsOutput(tegraStatsOuptut string) error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	for _, metricSender := range c.metricsSenders {
		err = metricSender.SendMetrics(sender, tegraStatsOuptut)
		if err != nil {
			return err
		}
	}
	sender.Commit()
	return nil
}

// Run executes the check
func (c *JetsonCheck) Run() error {
	tegraStatsCmd := fmt.Sprintf("%s %s", c.tegraStatsPath, strings.Join(c.commandOpts, " "))
	cmdStr := fmt.Sprintf("(%s) & pid=$!; (sleep %d && kill -9 $pid)", tegraStatsCmd, int((2 * tegraStatsInterval).Seconds()))
	var cmd *exec.Cmd
	if c.useSudo {
		// -n, non-interactive mode, no prompts are used
		cmd = exec.Command("sudo", "-n", "sh", "-c", cmdStr)
	} else {
		cmd = exec.Command("sh", "-c", cmdStr)
	}

	tegrastatsOutput, err := cmd.Output()
	if err != nil {
		switch err := err.(type) {
		case *exec.ExitError:
			if len(tegrastatsOutput) <= 0 {
				return fmt.Errorf("tegrastats did not produce any output: %s", err)
			}
			// We kill the process, so ExitError is expected - as long as
			// we got our output.
		default:
			return err
		}
	}
	log.Debugf("tegrastats output = %s\n", tegrastatsOutput)
	if err := c.processTegraStatsOutput(string(tegrastatsOutput)); err != nil {
		return fmt.Errorf("error processing tegrastats output: %s", err)
	}

	return nil
}

// Configure the GPU check
func (c *JetsonCheck) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	var conf checkCfg
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return err
	}
	if conf.TegraStatsPath != "" {
		c.tegraStatsPath = conf.TegraStatsPath
	} else {
		c.tegraStatsPath = "/usr/bin/tegrastats"
	}

	// We run tegrastats once and then kill the process. However, we set the interval to 500ms
	// because it will take tegrastats <interval> to produce its first output.
	c.commandOpts = []string{
		"--interval",
		strconv.FormatInt(tegraStatsInterval.Milliseconds(), 10),
	}

	c.useSudo = conf.UseSudo

	c.metricsSenders = []metricsSender{
		&cpuMetricSender{},
		&gpuMetricSender{},
		&iramMetricSender{},
		&ramMetricSender{},
		&swapMetricsSender{},
		&temperatureMetricsSender{},
		&voltageMetricsSender{},
	}

	for _, metricSender := range c.metricsSenders {
		err := metricSender.Init()
		if err != nil {
			return err
		}
	}

	return nil
}

func jetsonCheckFactory() check.Check {
	return &JetsonCheck{
		CheckBase: core.NewCheckBase(checkName),
	}
}

func init() {
	core.RegisterCheck(checkName, jetsonCheckFactory)
}
