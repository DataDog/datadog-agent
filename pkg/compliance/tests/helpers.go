// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tests

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/template"

	docker "github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/dynamic"
)

type suite struct {
	t        *testing.T
	hostname string
	rootDir  string

	dockerClient docker.CommonAPIClient
	auditClient  compliance.LinuxAuditClient
	kubeClient   dynamic.Interface

	rules []*assertedRule
}

type assertedRule struct {
	rootDir  string
	hostname string
	name     string
	input    string
	rego     string
	scope    string

	setups  []func(*testing.T, context.Context)
	asserts []func(*testing.T, *compliance.CheckEvent)
	events  []*compliance.CheckEvent

	noEvent   bool
	expectErr bool
}

func newTestBench(t *testing.T) *suite {
	rootDir := t.TempDir()
	return &suite{
		t:       t,
		rootDir: rootDir,
	}
}

func (s *suite) WithHostname(hostname string) *suite {
	s.hostname = hostname
	return s
}

func (s *suite) WithDockerClient(cl docker.CommonAPIClient) *suite {
	s.dockerClient = cl
	return s
}

func (s *suite) WithAuditClient(cl compliance.LinuxAuditClient) *suite {
	s.auditClient = cl
	return s
}

func (s *suite) WithKubeClient(cl dynamic.Interface) *suite {
	s.kubeClient = cl
	return s
}

func (s *suite) AddRule(name string) *assertedRule {
	for _, rule := range s.rules {
		if rule.name == name {
			s.t.Fatalf("rule with name %q already exist", name)
		}
	}
	rule := &assertedRule{
		name:     name,
		rootDir:  s.rootDir,
		hostname: s.hostname,
	}
	s.rules = append(s.rules, rule)
	return rule
}

func (s *suite) Run() {
	if len(s.rules) == 0 {
		s.t.Fatal("no rule to run")
	}
	for _, c := range s.rules {
		s.t.Run(c.name, func(t *testing.T) {
			options := compliance.ResolverOptions{
				Hostname: s.hostname,
			}
			if s.auditClient != nil {
				options.LinuxAuditProvider = func(ctx context.Context) (compliance.LinuxAuditClient, error) { return s.auditClient, nil }
			}
			if s.dockerClient != nil {
				options.DockerProvider = func(ctx context.Context) (docker.CommonAPIClient, error) { return s.dockerClient, nil }
			}
			if s.kubeClient != nil {
				options.KubernetesProvider = func(ctx context.Context) (dynamic.Interface, error) { return s.kubeClient, nil }
			}
			c.run(t, options)
		})
	}
}

func (s *suite) WriteTempFile(t *testing.T, data string) string {
	f, err := os.CreateTemp(s.rootDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func (c *assertedRule) Setup(setup func(t *testing.T, ctx context.Context)) *assertedRule {
	c.setups = append(c.setups, setup)
	return c
}

func (c *assertedRule) WriteFile(t *testing.T, name, data string) string {
	n := filepath.Join(c.rootDir, name)
	f, err := os.OpenFile(n, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fs.FileMode(0o644))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func (c *assertedRule) WithScope(scope string) *assertedRule {
	c.scope = scope
	return c
}

func (c *assertedRule) WithInput(input string, args ...any) *assertedRule {
	r := regexp.MustCompile(`(?m)^\t+`)
	input = r.ReplaceAllStringFunc(input, func(p string) string { return strings.Repeat("  ", len(p)) })
	c.input = strings.TrimSpace(fmt.Sprintf(input, args...))
	return c
}

func (c *assertedRule) WithRego(rego string, args ...any) *assertedRule {
	var buf bytes.Buffer
	rego = fmt.Sprintf(rego, args...)
	tmpl := template.Must(template.New("name").Parse(rego))
	err := tmpl.Execute(&buf, struct {
		Hostname string
		RuleID   string
	}{
		Hostname: c.hostname,
		RuleID:   c.name,
	})
	if err != nil {
		panic(err)
	}
	c.rego = buf.String()
	return c
}

func (c *assertedRule) AssertPassedEvent(f func(t *testing.T, evt *compliance.CheckEvent)) *assertedRule {
	c.asserts = append(c.asserts, func(t *testing.T, evt *compliance.CheckEvent) {
		if assert.Equal(t, compliance.CheckPassed, evt.Result) {
			if f != nil {
				f(t, evt)
			}
		} else {
			t.Logf("received unexpected %q event : %v", evt.Result, evt)
		}
	})
	return c
}

func (c *assertedRule) AssertFailedEvent(f func(t *testing.T, evt *compliance.CheckEvent)) *assertedRule {
	c.asserts = append(c.asserts, func(t *testing.T, evt *compliance.CheckEvent) {
		if assert.Equal(t, compliance.CheckFailed, evt.Result) {
			if f != nil {
				f(t, evt)
			}
		} else {
			t.Logf("received unexpected %q event : %v", evt.Result, evt)
		}
	})
	return c
}

func (c *assertedRule) AssertErrorEvent() *assertedRule {
	c.asserts = append(c.asserts, func(t *testing.T, evt *compliance.CheckEvent) {
		if assert.Equal(t, compliance.CheckError, evt.Result) {
			assert.NotNil(t, evt.Data["error"])
		}
	})
	return c
}

func (c *assertedRule) AssertNoEvent() *assertedRule {
	c.noEvent = true
	return c
}

func (c *assertedRule) AssertError() *assertedRule {
	c.expectErr = true
	return c
}

func (c *assertedRule) run(t *testing.T, options compliance.ResolverOptions) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, setup := range c.setups {
		setup(t, ctx)
	}

	suiteName := strings.ReplaceAll(c.name, string(os.PathSeparator), "")
	suiteData := buildSuite(suiteName, c)

	_ = c.WriteFile(t, suiteName+".rego", c.rego)
	_ = c.WriteFile(t, suiteName+".yaml", suiteData)

	benchmarks, err := compliance.LoadBenchmarks(c.rootDir, suiteName+".yaml", nil)
	if c.expectErr {
		if err == nil {
			t.Fatalf("expected to fail running checks but resulting in no error")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(benchmarks) != 1 {
		t.Fatalf("expected to load one benchmark, found %d", len(benchmarks))
	}

	if c.noEvent && len(c.asserts) > 0 {
		t.Fatalf("no event expected: asserts should be empty")
	}
	if !c.noEvent && len(c.asserts) == 0 {
		t.Fatalf("missing assertions")
	}

	resolver := compliance.NewResolver(ctx, options)
	defer resolver.Close()
	benchmark := benchmarks[0]
	for _, rule := range benchmark.Rules {
		inputs, err := resolver.ResolveInputs(ctx, rule)
		var events []*compliance.CheckEvent
		if err != nil {
			events = append(events, compliance.CheckEventFromError(compliance.RegoEvaluator, rule, benchmark, err))
		} else {
			events = compliance.EvaluateRegoRule(ctx, inputs, benchmark, rule)
		}
		if c.noEvent {
			if len(events) > 0 {
				for _, event := range events {
					t.Logf("unexpected event: %+v", event)
				}
				t.Fatalf("expected no event on this rule: received %d", len(events))
			}
		} else if len(events) != len(c.asserts) {
			t.Logf("expected %d events but received %d", len(c.asserts), len(events))
			t.Fail()
		}

		for i, event := range events {
			if i < len(c.asserts) {
				c.asserts[i](t, event)
			} else {
				t.Logf("unexpected event %d", i)
				t.Fail()
			}
		}
	}
}

func (c *assertedRule) Report(event *compliance.CheckEvent) {
	c.events = append(c.events, event)
}

func buildSuite(name string, rules ...*assertedRule) string {
	const suiteTpl = `schema:
  version: 1.0.0
name: %s
framework: %s
version: %s
rules:`
	const ruleTpl = `id: %s
version: 123
scope:
  - %s
input:
  %s`

	suite := fmt.Sprintf(suiteTpl, name, "framework_"+name, "42.12")
	for _, rule := range rules {
		scope := rule.scope
		if scope == "" {
			scope = "none"
		}
		ruleData := fmt.Sprintf(ruleTpl, rule.name, scope, indent(1, rule.input))
		suite += "\n  - " + indent(2, ruleData)
	}
	return suite
}

func indent(count int, s string) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, strings.Repeat("  ", count)))
}
