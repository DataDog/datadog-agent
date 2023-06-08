// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !libopenscap || !cgo || !linux

package compliance

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance/metrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	XCCDF_RESULT_PASS = iota + 1
	XCCDF_RESULT_FAIL
	XCCDF_RESULT_ERROR
	XCCDF_RESULT_UNKNOWN
	XCCDF_RESULT_NOT_APPLICABLE
	XCCDF_RESULT_NOT_CHECKED
	XCCDF_RESULT_NOT_SELECTED
)

type oscapIORule struct {
	Profile string
	Rule    string
}

type oscapIOResult struct {
	Rule   string
	Result int
}

var (
	oscapIOsMu sync.Mutex
	oscapIOs   = map[string]*oscapIO{}
)

type oscapIO struct {
	cmd      *exec.Cmd
	File     string
	RuleCh   chan *oscapIORule
	ResultCh chan *oscapIOResult
	ErrorCh  chan error
}

// From pkg/collector/corechecks/embed/process_agent.go.
func getOSCAPIODefaultBinPath() (string, error) {
	here, _ := executable.Folder()
	binPath := filepath.Join(here, "..", "..", "embedded", "bin", "oscap-io")
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}
	return binPath, fmt.Errorf("Can't access the default oscap-io binary at %s", binPath)
}

func newOSCAPIO(file string) *oscapIO {
	return &oscapIO{
		File:     file,
		RuleCh:   make(chan *oscapIORule, 0),
		ResultCh: make(chan *oscapIOResult, 0),
		ErrorCh:  make(chan error, 0),
	}
}

func (p *oscapIO) Run(ctx context.Context) error {
	defer p.Stop()

	if config.IsContainerized() {
		hostRoot := os.Getenv("HOST_ROOT")
		if hostRoot == "" {
			hostRoot = "/host"
		}

		os.Setenv("OSCAP_PROBE_ROOT", hostRoot)
		defer os.Unsetenv("OSCAP_PROBE_ROOT")
	}

	args := []string{}
	args = append(args, p.File)

	binPath, err := getOSCAPIODefaultBinPath()
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Dir = filepath.Dir(p.File)
	p.cmd = cmd

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	defer stdout.Close()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	defer stderr.Close()

	log.Debugf("Executing %s in %s\n", cmd.String(), cmd.Dir)

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	go func() {
		for scanner.Scan() {
			log.Debugf("<- %s", scanner.Text())
			line := strings.Split(scanner.Text(), " ")
			if len(line) != 2 {
				p.ErrorCh <- fmt.Errorf("invalid output: %v", line)
				continue
			}
			result, err := strconv.Atoi(line[1])
			if err != nil {
				p.ErrorCh <- fmt.Errorf("strconv.Atoi '%s': %v", line[1], err)
				continue
			}
			p.ResultCh <- &oscapIOResult{Rule: line[0], Result: result}
		}
	}()

	scannerErr := bufio.NewScanner(stderr)
	go func() {
		for scannerErr.Scan() {
			log.Warnf("error: %v", scannerErr.Text())
		}
	}()

	go func() {
		for {
			rule := <-p.RuleCh
			if rule == nil {
				return
			}
			log.Debugf("-> %s %s\n", rule.Profile, rule.Rule)
			_, err := io.WriteString(stdin, rule.Profile+" "+rule.Rule+"\n")
			if err != nil {
				log.Warnf("error writing string '%s %s': %v", rule.Profile, rule.Rule, err)
				return
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		log.Warnf("process exited: %v", err)
		return err
	}

	return nil
}

func (p *oscapIO) Stop() {
	oscapIOsMu.Lock()
	defer oscapIOsMu.Unlock()
	oscapIOs[p.File] = nil
	close(p.RuleCh)
	close(p.ResultCh)
	close(p.ErrorCh)
}

func EvaluateXCCDFRule(ctx context.Context, hostname string, statsdClient *statsd.Client, benchmark *Benchmark, rule *Rule) []*CheckEvent {
	if !rule.IsXCCDF() {
		log.Errorf("given rule is not an XCCDF rule %s", rule.ID)
		return nil
	}
	return evaluateXCCDFRule(ctx, hostname, statsdClient, benchmark, rule, rule.InputSpecs[0].XCCDF)
}

func evaluateXCCDFRule(ctx context.Context, hostname string, statsdClient *statsd.Client, benchmark *Benchmark, rule *Rule, spec *InputSpecXCCDF) []*CheckEvent {
	oscapIOsMu.Lock()
	file := filepath.Join(benchmark.dirname, spec.Name)
	p := oscapIOs[file]
	if p == nil {
		p = newOSCAPIO(file)
		oscapIOs[file] = p
		go func() {
			err := p.Run(ctx)
			if err != nil {
				log.Warnf("Run: %v", err)
			}
		}()
	}
	oscapIOsMu.Unlock()

	var reqs []*oscapIORule
	if len(spec.Rules) > 0 {
		for _, rule := range spec.Rules {
			reqs = append(reqs, &oscapIORule{Profile: spec.Profile, Rule: rule})
		}
	} else if (spec.Rule) != "" {
		reqs = append(reqs, &oscapIORule{Profile: spec.Profile, Rule: spec.Rule})
	}

	start := time.Now()
	for _, req := range reqs {
		select {
		case <-ctx.Done():
			return nil
		case p.RuleCh <- req:
		}
	}

	var events []*CheckEvent
	c := time.After(60 * time.Second)
	for i := 0; i < len(reqs); i++ {
		select {
		case <-ctx.Done():
			return nil
		case <-c:
			log.Warnf("timed out waiting for expected results")
		case err := <-p.ErrorCh:
			log.Warnf("error: %v", err)
			events = append(events, NewCheckError(XCCDFEvaluator, err, hostname, "host", rule, benchmark))
		case ruleResult := <-p.ResultCh:
			if ruleResult == nil {
				return nil
			}
			var event *CheckEvent
			switch ruleResult.Result {
			case XCCDF_RESULT_PASS:
				event = NewCheckEvent(XCCDFEvaluator, CheckPassed, nil, hostname, "host", rule, benchmark)
			case XCCDF_RESULT_FAIL:
				event = NewCheckEvent(XCCDFEvaluator, CheckFailed, nil, hostname, "host", rule, benchmark)
			case XCCDF_RESULT_ERROR, XCCDF_RESULT_UNKNOWN:
				errReason := fmt.Errorf("XCCDF_RESULT_ERROR")
				event = NewCheckError(XCCDFEvaluator, errReason, hostname, "host", rule, benchmark)
			case XCCDF_RESULT_NOT_APPLICABLE:
				skipReason := fmt.Errorf("XCCDF_RESULT_NOT_APPLICABLE")
				event = NewCheckSkipped(XCCDFEvaluator, skipReason, hostname, "host", rule, benchmark)
			case XCCDF_RESULT_NOT_CHECKED, XCCDF_RESULT_NOT_SELECTED:
			}
			if event != nil {
				events = append(events, event)
			}
		}
	}

	if statsdClient != nil {
		tags := []string{
			"rule_id:" + rule.ID,
			"rule_input_type:xccdf",
			"agent_version:" + version.AgentVersion,
		}
		if err := statsdClient.Count(metrics.MetricInputsHits, int64(len(reqs)), tags, 1.0); err != nil {
			log.Errorf("failed to send input metric: %v", err)
		}
		if err := statsdClient.Timing(metrics.MetricInputsDuration, time.Since(start), tags, 1.0); err != nil {
			log.Errorf("failed to send input metric: %v", err)
		}
	}

	return events
}
