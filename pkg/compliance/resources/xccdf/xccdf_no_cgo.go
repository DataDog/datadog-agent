// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !libopenscap || !cgo || !linux
// +build !libopenscap !cgo !linux

package xccdf

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

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

var mu sync.Mutex

type Rule struct {
	Profile string
	Rule    string
}

type Result struct {
	Rule   string
	Result int
}

var processes = map[string]*Process{}

type Process struct {
	cmd      *exec.Cmd
	Name     string
	File     string
	Dir      string
	RuleCh   chan *Rule
	ResultCh chan *Result
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

func newProcess(name string, dir string) *Process {
	return &Process{
		Name:     name,
		Dir:      dir,
		File:     filepath.Join(dir, name),
		RuleCh:   make(chan *Rule, 0),
		ResultCh: make(chan *Result, 0),
		ErrorCh:  make(chan error, 0),
	}
}

func (p *Process) Run() error {
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

	cmd := exec.Command(binPath, args...)
	cmd.Dir = p.Dir
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
			p.ResultCh <- &Result{Rule: line[0], Result: result}
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

func (p *Process) Stop() {
	mu.Lock()
	defer mu.Unlock()
	processes[p.Name] = nil
	close(p.RuleCh)
	close(p.ResultCh)
	close(p.ErrorCh)
}

func resolve(_ context.Context, e env.Env, id string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	mu.Lock()
	p := processes[res.Xccdf.Name]
	if p == nil {
		p = newProcess(res.Xccdf.Name, e.ConfigDir())
		processes[res.Xccdf.Name] = p
		go func() {
			err := p.Run()
			if err != nil {
				log.Warnf("Run: %v", err)
			}
		}()
	}
	mu.Unlock()

	var rules []string
	if res.Xccdf.Rule != "" {
		rules = []string{res.Xccdf.Rule}
	} else {
		rules = res.Xccdf.Rules
	}

	for _, rule := range rules {
		p.RuleCh <- &Rule{Profile: res.Xccdf.Profile, Rule: rule}
	}

	var instances []resources.ResolvedInstance

	c := time.After(60 * time.Second)

	for i := 0; i < len(rules); i++ {
		select {
		case <-c:
			log.Warnf("timed out waiting for expected results")
		case err := <-p.ErrorCh:
			log.Warnf("error: %v", err)
			return nil, err
		case ruleResult := <-p.ResultCh:
			if ruleResult == nil {
				return nil, nil
			}
			var result string
			switch ruleResult.Result {
			case XCCDF_RESULT_PASS:
				result = "passed"
			case XCCDF_RESULT_FAIL:
				result = "failing"
			case XCCDF_RESULT_ERROR, XCCDF_RESULT_UNKNOWN:
				result = "error"
			case XCCDF_RESULT_NOT_APPLICABLE:
			case XCCDF_RESULT_NOT_CHECKED, XCCDF_RESULT_NOT_SELECTED:
			}
			if result != "" {
				instances = append(instances, resources.NewResolvedInstance(
					eval.NewInstance(eval.VarMap{}, eval.FunctionMap{}, eval.RegoInputMap{
						"name":   e.Hostname(),
						"result": result,
						"rule":   ruleResult.Rule,
					}), e.Hostname(), "host"))
			}
		}
	}

	return resources.NewResolvedInstances(instances), nil
}
