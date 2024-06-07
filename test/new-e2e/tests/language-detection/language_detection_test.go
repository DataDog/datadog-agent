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
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

//go:embed etc/process_config.yaml
var configStr string

type languageDetectionSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLanguageDetectionSuite(t *testing.T) {
	agentParams := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(configStr),
	}

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentParams...))),
	}

	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, e2e.WithDevMode())
	}

	e2e.Run(t, &languageDetectionSuite{}, options...)
}

func (s *languageDetectionSuite) checkDetectedLanguage(command string, language string) {
	var pid string
	require.Eventually(s.T(),
		func() bool {
			pid = s.getPidForCommand(command)
			return len(pid) > 0
		},
		10*time.Second, 10*time.Millisecond,
		fmt.Sprintf("pid not found for command %s", command),
	)

	var actualLanguage string
	var err error
	assert.Eventually(s.T(),
		func() bool {
			actualLanguage, err = s.getLanguageForPid(pid)
			return err == nil && actualLanguage == language
		},
		10*time.Second, 100*time.Millisecond,
		fmt.Sprintf("language match not found, pid = %s, expected = %s, actual = %s, err = %v",
			pid, language, actualLanguage, err),
	)

	s.Env().RemoteHost.MustExecute(fmt.Sprintf("kill -SIGTERM %s", pid))
}

func (s *languageDetectionSuite) getPidForCommand(command string) string {
	pid, err := s.Env().RemoteHost.Execute(fmt.Sprintf("ps -C %s -o pid=", command))
	if err != nil {
		return ""
	}
	pid = strings.TrimSpace(pid)
	// special handling in case multiple commands match
	pids := strings.Split(pid, "\n")
	return pids[0]
}

func (s *languageDetectionSuite) getLanguageForPid(pid string) (string, error) {
	wl := s.Env().RemoteHost.MustExecute("sudo /opt/datadog-agent/bin/agent/agent workload-list")
	if len(strings.TrimSpace(wl)) == 0 {
		return "", errors.New("agent workload-list was empty")
	}

	scanner := bufio.NewScanner(strings.NewReader(wl))
	pidLine := fmt.Sprintf("PID: %s", pid)
	for scanner.Scan() {
		line := scanner.Text()
		if line == pidLine {
			scanner.Scan() // nspid
			scanner.Scan() // container id
			scanner.Scan() // creation time
			scanner.Scan() // language
			return scanner.Text()[len("Language: "):], nil
		}
	}

	return "", errors.New("no language found")
}
