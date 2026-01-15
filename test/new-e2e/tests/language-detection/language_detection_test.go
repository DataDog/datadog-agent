// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

//go:embed etc/process_config.yaml
var processConfigStr string

//go:embed etc/core_config.yaml
var coreConfigStr string

//go:embed etc/process_config_no_check.yaml
var processConfigNoCheckStr string

//go:embed etc/core_config_no_check.yaml
var coreConfigNoCheckStr string

type languageDetectionSuite struct {
	e2e.BaseSuite[environments.Host]
}

func getProvisionerOptions(agentParams []func(*agentparams.Params) error) []awshost.ProvisionerOption {
	return []awshost.ProvisionerOption{
		awshost.WithRunOptions(
			ec2.WithAgentOptions(agentParams...),
			ec2.WithEC2InstanceOptions(ec2.WithAMI("ami-090c309e8ced8ecc2", os.Ubuntu2204, os.AMD64Arch)),
		),
	}
}

func TestLanguageDetectionSuite(t *testing.T) {
	agentParams := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(processConfigStr),
	}

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
			getProvisionerOptions(agentParams)...,
		)),
	}

	e2e.Run(t, &languageDetectionSuite{}, options...)
}

func (s *languageDetectionSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer s.CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	s.installPython()
	s.installPHP()
}

func (s *languageDetectionSuite) checkDetectedLanguage(pid string, language string, source string) {
	s.Env().RemoteHost.MustExecute("kill -0 " + pid) // check PID refers to an existing, signalable process

	var actualLanguage string
	var err error
	assert.Eventually(s.T(),
		func() bool {
			actualLanguage, err = s.getLanguageForPid(pid, source)
			return err == nil && actualLanguage == language
		},
		60*time.Second, 100*time.Millisecond,
		fmt.Sprintf("language match not found, pid = %s, expected = %s, actual = %s, err = %v",
			pid, language, actualLanguage, err),
	)

	s.Env().RemoteHost.MustExecute("kill -SIGTERM " + pid)
}

func (s *languageDetectionSuite) getLanguageForPid(pid string, source string) (string, error) {
	wl := s.Env().RemoteHost.MustExecute("sudo /opt/datadog-agent/bin/agent/agent workload-list")
	if len(strings.TrimSpace(wl)) == 0 {
		return "", errors.New("agent workload-list was empty")
	}

	scanner := bufio.NewScanner(strings.NewReader(wl))
	headerLine := fmt.Sprintf("=== Entity process sources(merged):[%s] id: %s ===", source, pid)

	for scanner.Scan() {
		line := scanner.Text()
		if line == headerLine {
			scanner.Scan() // entity line
			scanner.Scan() // pid
			scanner.Scan() // name
			scanner.Scan() // exe
			scanner.Scan() // cmdline
			scanner.Scan() // nspid
			scanner.Scan() // container id
			scanner.Scan() // creation time
			scanner.Scan() // language
			return scanner.Text()[len("Language: "):], nil
		}
	}

	return "", errors.New("no language found")
}
