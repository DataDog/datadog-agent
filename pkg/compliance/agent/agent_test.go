// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package agent

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type tempEnv struct {
	dir  string
	prev string
}

func (e *tempEnv) leave() {
	defer os.Chdir(e.prev)
}

func enterTempEnv(t *testing.T) *tempEnv {
	t.Helper()
	assert := assert.New(t)
	tempDir, err := ioutil.TempDir("", "compliance-agent-")
	assert.NoError(err)

	err = util.CopyDir("./testdata/configs", tempDir)
	assert.NoError(err)

	files, err := filepath.Glob(filepath.Join(tempDir, "files/*"))
	assert.NoError(err)
	for _, file := range files {
		_ = os.Chmod(file, 0644)
	}

	prev, _ := os.Getwd()
	_ = os.Chdir(tempDir)

	os.Setenv("KUBERNETES", "yes")
	return &tempEnv{
		dir:  tempDir,
		prev: prev,
	}
}

type eventMatch struct {
	ruleID       string
	resourceID   string
	resourceType string
	result       string
	path         string
	permissions  uint64
}

func eventMatcher(m eventMatch) interface{} {
	return func(e *event.Event) bool {
		if e.AgentRuleID != m.ruleID ||
			e.Result != m.result ||
			e.ResourceID != m.resourceID ||
			e.ResourceType != m.resourceType {
			return false
		}

		if e.Data == nil {
			return false
		}

		eventData := e.Data.(event.Data)
		return eventData["file.path"] == m.path && eventData["file.permissions"] == m.permissions
	}
}

func TestRun(t *testing.T) {
	assert := assert.New(t)

	e := enterTempEnv(t)
	defer e.leave()

	reporter := &mocks.Reporter{}

	reporter.On(
		"Report",
		mock.MatchedBy(
			eventMatcher(
				eventMatch{
					ruleID:       "cis-docker-1",
					resourceID:   "the-host",
					resourceType: "docker",
					result:       "passed",
					path:         "/files/daemon.json",
					permissions:  0644,
				},
			),
		),
	).Once()

	reporter.On(
		"Report",
		mock.MatchedBy(
			eventMatcher(
				eventMatch{
					ruleID:       "cis-kubernetes-1",
					resourceID:   "the-host",
					resourceType: "kubernetesNode",
					result:       "failed",
					path:         "/files/kube-apiserver.yaml",
					permissions:  0644,
				},
			),
		),
	).Once()

	defer reporter.AssertExpectations(t)

	scheduler := &mocks.Scheduler{}
	defer scheduler.AssertExpectations(t)

	scheduler.On("Run").Once().Return(nil)
	scheduler.On("Stop").Once().Return(nil)

	scheduler.On("Enter", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		check := args.Get(0).(check.Check)
		check.Run()
	})

	dockerClient := &mocks.DockerClient{}
	dockerClient.On("Close").Return(nil).Once()
	defer dockerClient.AssertExpectations(t)

	nodeLabels := map[string]string{
		"node-role.kubernetes.io/worker": "",
	}

	agent, err := New(
		reporter,
		scheduler,
		e.dir,
		checks.WithHostname("the-host"),
		checks.WithHostRootMount(e.dir),
		checks.WithDockerClient(dockerClient),
		checks.WithNodeLabels(nodeLabels),
	)
	assert.NoError(err)

	err = agent.Run()
	assert.NoError(err)
	agent.Stop()

	st := agent.builder.GetCheckStatus()
	assert.Len(st, 2)
	assert.Equal("cis-docker-1", st[0].RuleID)
	assert.Equal("passed", st[0].LastEvent.Result)

	assert.Equal("cis-kubernetes-1", st[1].RuleID)
	assert.Equal("failed", st[1].LastEvent.Result)

	v, err := json.Marshal(st)
	assert.NoError(err)

	// Check the expvar value
	assert.JSONEq(string(v), status.Get("Checks").String())
}

func TestRunChecks(t *testing.T) {
	assert := assert.New(t)

	e := enterTempEnv(t)
	defer e.leave()

	reporter := &mocks.Reporter{}

	reporter.On(
		"Report",
		mock.MatchedBy(
			eventMatcher(
				eventMatch{
					ruleID:       "cis-docker-1",
					resourceID:   "the-host",
					resourceType: "docker",
					result:       "passed",
					path:         "/files/daemon.json",
					permissions:  0644,
				},
			),
		),
	).Once()

	defer reporter.AssertExpectations(t)

	dockerClient := &mocks.DockerClient{}
	dockerClient.On("Close").Return(nil).Once()
	defer dockerClient.AssertExpectations(t)

	err := RunChecks(
		reporter,
		e.dir,
		checks.WithMatchSuite(checks.IsFramework("cis-docker")),
		checks.WithMatchRule(checks.IsRuleID("cis-docker-1")),
		checks.WithHostname("the-host"),
		checks.WithHostRootMount(e.dir),
		checks.WithDockerClient(dockerClient),
	)
	assert.NoError(err)
}

func TestRunChecksFromFile(t *testing.T) {
	assert := assert.New(t)
	e := enterTempEnv(t)
	defer e.leave()

	reporter := &mocks.Reporter{}

	reporter.On(
		"Report",
		mock.MatchedBy(
			eventMatcher(
				eventMatch{
					ruleID:       "cis-kubernetes-1",
					resourceID:   "the-host",
					resourceType: "kubernetesNode",
					result:       "failed",
					path:         "/files/kube-apiserver.yaml",
					permissions:  0644,
				},
			),
		),
	).Once()

	defer reporter.AssertExpectations(t)

	dockerClient := &mocks.DockerClient{}
	dockerClient.On("Close").Return(nil).Once()
	defer dockerClient.AssertExpectations(t)

	nodeLabels := map[string]string{
		"node-role.kubernetes.io/worker": "",
	}

	err := RunChecksFromFile(
		reporter,
		filepath.Join(e.dir, "cis-kubernetes.yaml"),
		checks.WithHostname("the-host"),
		checks.WithHostRootMount(e.dir),
		checks.WithDockerClient(dockerClient),
		checks.WithNodeLabels(nodeLabels),
	)
	assert.NoError(err)
}
