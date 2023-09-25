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
	"github.com/DataDog/datadog-agent/pkg/compliance/scap"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	enableSysChar = false
)

const (
	//revive:disable
	XCCDF_RESULT_PASS = iota + 1
	XCCDF_RESULT_FAIL
	XCCDF_RESULT_ERROR
	XCCDF_RESULT_UNKNOWN
	XCCDF_RESULT_NOT_APPLICABLE
	XCCDF_RESULT_NOT_CHECKED
	XCCDF_RESULT_NOT_SELECTED
	//revive:enable
)

type oscapIORule struct {
	Profile string
	Rule    string
}

type oscapIOResult struct {
	Rule   string
	Result int
	Data   map[string]interface{}
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
	DoneCh   chan bool
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
		RuleCh:   make(chan *oscapIORule),
		ResultCh: make(chan *oscapIOResult),
		ErrorCh:  make(chan error),
		DoneCh:   make(chan bool),
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
	if enableSysChar {
		args = append(args, "-syschar")
	}
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

	r := bufio.NewReader(stdout)
	go func() {
		for {
			s, err := r.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				p.ErrorCh <- fmt.Errorf("error reading line: '%v'", s)
				continue
			}
			s = strings.TrimRight(s, "\n")

			if s == "" {
				continue
			}

			log.Debugf("<- %s", s)
			line := strings.Split(s, " ")
			if len(line) != 2 {
				p.ErrorCh <- fmt.Errorf("invalid output: '%v'", line)
				continue
			}
			result, err := strconv.Atoi(line[1])
			if err != nil {
				p.ErrorCh <- fmt.Errorf("strconv.Atoi '%s': %v", line[1], err)
				continue
			}

			data := make(map[string]interface{}, 0)

			if enableSysChar {
				doc, err := scap.ReadDocument(r)
				if err != nil {
					p.ErrorCh <- fmt.Errorf("scap.ReadDocument: %v", err)
					continue
				}

				syschar, err := scap.SysChar(doc)
				if err != nil {
					p.ErrorCh <- fmt.Errorf("scap.SysChar: %v", err)
					continue
				}

				data["system_characteristics"] = syschar
			}

			p.ResultCh <- &oscapIOResult{Rule: line[0], Result: result, Data: data}
		}
	}()

	scannerErr := bufio.NewScanner(stderr)
	go func() {
		for scannerErr.Scan() {
			log.Warnf("error: %v", scannerErr.Text())
		}
	}()

	// Stop oscap-io process after 60 minutes of inactivity.
	timeout := 60 * time.Minute

	go func() {
		t := time.NewTimer(timeout)
		for {
			select {
			case <-p.DoneCh:
				// The oscap-io process has been terminated.
				return
			case <-t.C:
				log.Warnf("oscap-io has been inactive for %s; exiting", timeout)
				err := p.Kill()
				if err != nil {
					log.Warnf("failed to kill process: %v", err)
				}
				return
			case rule := <-p.RuleCh:
				if rule == nil {
					return
				}
				log.Debugf("-> %s %s\n", rule.Profile, rule.Rule)
				_, err := io.WriteString(stdin, rule.Profile+" "+rule.Rule+"\n")
				if err != nil {
					log.Warnf("error writing string '%s %s': %v", rule.Profile, rule.Rule, err)
					return
				}
				t.Reset(timeout)
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		log.Debugf("process exited: %v", err)
		return err
	}

	return nil
}

func (p *oscapIO) Stop() {
	oscapIOsMu.Lock()
	defer oscapIOsMu.Unlock()
	delete(oscapIOs, p.File)
	close(p.DoneCh)
}

func (p *oscapIO) Kill() error {
	if err := p.cmd.Process.Kill(); err != nil {
		return err
	}
	return nil
}

// EvaluateXCCDFRule evaluates the given rule using OpenSCAP tool.
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
				log.Debugf("Run: %v", err)
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
		case <-p.DoneCh:
			// The oscap-io process has been terminated.
			for _, req := range reqs {
				log.Warnf("dropping rule '%s %s'", req.Profile, req.Rule)
			}
			close(p.RuleCh)
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
			log.Warnf("timed out waiting for expected results for rule %s", reqs[i].Rule)
			// If no result has been received, it's likely for the oscap-io process to be stuck, so we kill it.
			oscapIOsMu.Lock()
			delete(oscapIOs, p.File)
			oscapIOsMu.Unlock()
			err := p.Kill()
			if err != nil {
				log.Warnf("failed to kill process: %v", err)
			}
			return nil
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
				event = NewCheckEvent(XCCDFEvaluator, CheckPassed, ruleResult.Data, hostname, "host", rule, benchmark)
			case XCCDF_RESULT_FAIL:
				event = NewCheckEvent(XCCDFEvaluator, CheckFailed, ruleResult.Data, hostname, "host", rule, benchmark)
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

// FinishXCCDFBenchmark finishes an XCCDF benchmark by terminating the oscap-io processes.
func FinishXCCDFBenchmark(ctx context.Context, benchmark *Benchmark) {
	oscapIOsMu.Lock()
	defer oscapIOsMu.Unlock()

	if len(oscapIOs) == 0 {
		// No oscap-io process is running.
		return
	}

	for _, rule := range benchmark.Rules {
		if !rule.IsXCCDF() {
			continue
		}
		if len(benchmark.Rules[0].InputSpecs) == 0 {
			continue
		}
		file := filepath.Join(benchmark.dirname, rule.InputSpecs[0].XCCDF.Name)
		p := oscapIOs[file]
		if p == nil {
			continue
		}
		delete(oscapIOs, file)
		err := p.Kill()
		if err != nil {
			log.Warnf("failed to kill process: %v", err)
		}
		if len(oscapIOs) == 0 {
			// If no oscap-io process is running, we don't have to loop through every rules.
			return
		}
	}
}
