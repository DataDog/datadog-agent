// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logagent

import (
	_ "embed"
	"fmt"
	"os"
	"testing"
	"time"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	e2e "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

//go:embed log-config/utf-log-config.yaml
var utfLittleEndianLogConfig []byte

//go:embed log-config/utf-be-log-config.yaml
var utfBigEndianLogConfig []byte

// LinuxVMFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type utfSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
	DevMode bool
}

// TestE2EVMFakeintakeSuite runs the E2E test suite for the log agent with a VM and fake intake.
func TestUtfSuite(t *testing.T) {
	s := &utfSuite{}
	_, s.DevMode = os.LookupEnv("TESTS_E2E_DEVMODE")

	e2e.Run(t, s, e2e.FakeIntakeStackDef())
}

func (s *utfSuite) TestUtfTailing() {
	// Run test cases
	s.Run("UtfLittleEndianCollection", s.UtfLittleEndianCollection)
	// s.Run("UtfBigEndianCollection", s.UtfBigEndianCollection)
}

func (s *utfSuite) UtfBigEndianCollection() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(
			agentparams.WithLogs(),
			agentparams.WithIntegration("custom_logs.d", string(utfBigEndianLogConfig)))))
	t := s.T()
	fakeintake := s.Env().Fakeintake

	// Create a new log file with permissionn inaccessable to the agent
	s.Env().VM.Execute("sudo touch /var/log/hello-world-utf.log")
	s.Env().VM.Execute("sudo chmod 666 /var/log/hello-world-utf.log")

	// Part 1: Ensure no logs are present in fakeintake
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := fakeintake.FilterLogs("utfservice")
		if !assert.NoError(c, err, "Unable to filter logs by the service 'hello'.") {
			return
		}
		// If logs are found, print their content for debugging
		if !assert.Empty(c, logs, "Logs were found when none were expected.") {
			cat, _ := s.Env().VM.ExecuteWithError("cat /var/log/hello-world-utf.log")
			t.Logf("Logs detected when none were expected: %v", cat)
		}
	}, 2*time.Minute, 10*time.Second)

	// send utf16 log
	utfLogCommand := `python3 -c "f = open('/var/log/hello-world-utf.log', 'ab'); t = 'big endian log\n'.encode('utf-16-be'); f.write(t); f.close()"`
	s.Env().VM.Execute(utfLogCommand)

	expectLogs := true
	service := "utfservice"

	content := "big endian log"
	// check that intake has log
	s.EventuallyWithT(func(c *assert.CollectT) {
		names, err := fakeintake.GetLogServiceNames()
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}
		fmt.Println(names)
		logs, err := fakeintake.FilterLogs(service, fi.WithMessageMatching(content))
		intakeLogs := logsToString(logs)
		assert.NoErrorf(c, err, "Error found: %s", err)
		if expectLogs {
			t.Logf("Logs with content: '%s' service: %s found", content, names)
			fmt.Println(intakeLogs, "rz")
			assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', but received %s logs.", content, intakeLogs)
		} else {
			t.Logf("No logs with content: '%s' service: %s found as expected", content, names)
			assert.Empty(c, logs, "No logs with content: '%s' is expected to be found from service instead found: %s", content, intakeLogs)
		}
		fmt.Println(len(intakeLogs), "len utf-16-le")
		fmt.Println(intakeLogs)

		logs, _ = fakeintake.FilterLogs(service)
		fmt.Println(len(logs), "total", logs)
	}, 2*time.Minute, 10*time.Second)

	s.EventuallyWithT(func(c *assert.CollectT) {
		err := s.Env().Fakeintake.FlushServerAndResetAggregators()
		assert.NoErrorf(c, err, "Having issue flushing server and resetting aggregators, retrying...")
	}, 5*time.Minute, 10*time.Second)

	s.Env().VM.Execute("sudo rm -f /var/log/hello-world-utf.log")
}

func (s *utfSuite) UtfLittleEndianCollection() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(
			agentparams.WithLogs(),
			agentparams.WithIntegration("custom_logs.d", string(utfLittleEndianLogConfig)))))
	t := s.T()
	fakeintake := s.Env().Fakeintake

	// Create a new log file with permissionn inaccessable to the agent
	s.Env().VM.Execute("sudo touch /var/log/hello-world-utf.log")
	s.Env().VM.Execute("sudo chmod 666 /var/log/hello-world-utf.log")

	// send utf16 log
	utfLogCommand := `python3 -c "f = open('/var/log/hello-world-utf.log', 'ab'); t = 'This is just sample text encoded in utf-16\n'.encode('utf-16'); f.write(t); f.close()"`
	s.Env().VM.Execute(utfLogCommand)

	expectLogs := true
	service := "utfservice"

	content := "This is just sample text encoded in utf-16"
	// check that intake has log
	s.EventuallyWithT(func(c *assert.CollectT) {
		names, err := fakeintake.GetLogServiceNames()
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}
		fmt.Println(names)
		logs, err := fakeintake.FilterLogs(service, fi.WithMessageMatching(content))
		intakeLogs := logsToString(logs)
		assert.NoErrorf(c, err, "Error found: %s", err)
		if expectLogs {
			t.Logf("Logs with content: '%s' service: %s found", content, names)
			fmt.Println(intakeLogs, "rz")
			assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', but received %s logs.", content, intakeLogs)
		} else {
			t.Logf("No logs with content: '%s' service: %s found as expected", content, names)
			assert.Empty(c, logs, "No logs with content: '%s' is expected to be found from service instead found: %s", content, intakeLogs)
		}
		fmt.Println(len(intakeLogs), "len utf-16-le")
		fmt.Println(intakeLogs)

		logs, _ = fakeintake.FilterLogs(service)
		fmt.Println(len(logs), "total", logs)
	}, 2*time.Minute, 10*time.Second)

	fmt.Println("reached here")
	s.EventuallyWithT(func(c *assert.CollectT) {
		fmt.Println("flushing fake intake and resetting aggregator...")
		err := s.Env().Fakeintake.FlushServerAndResetAggregators()
		assert.NoErrorf(c, err, "Having issue flushing server and resetting aggregators, retrying...")
	}, 5*time.Minute, 10*time.Second)

	s.Env().VM.Execute("sudo rm -f /var/log/hello-world-utf.log")
}
