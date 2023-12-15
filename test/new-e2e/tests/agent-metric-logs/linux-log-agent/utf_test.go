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

//go:embed log-config/utf-16-le-log-config.yaml
var utfLittleEndianLogConfig []byte

//go:embed log-config/utf-16-be-log-config.yaml
var utfBigEndianLogConfig []byte

var service = "utfservice"

// UTF defines a test suite for the log agent interacting with a virtual machine and fake intake.
type UtfSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
	DevMode bool
}

// TestE2EVMFakeintakeSuite runs the E2E test suite for the log agent with a VM and fake intake.
func TestUtfSuite(t *testing.T) {
	s := &UtfSuite{}
	_, s.DevMode = os.LookupEnv("TESTS_E2E_DEVMODE")
	e2e.Run(t, s, e2e.FakeIntakeStackDef())
}

func (s *UtfSuite) generateUtfLog(endianness, content string) error {
	utfLogGenerationCommand := fmt.Sprintf(`sudo python3 -c "f = open('/var/log/hello-world-utf.log', 'ab'); t = '%s\n'.encode('utf-16-%s'); f.write(t); f.close()"`, content, endianness)
	_, err := s.Env().VM.ExecuteWithError(utfLogGenerationCommand)
	return err
}

func (s *UtfSuite) BeforeTest(_, _ string) {
	s.Suite.BeforeTest("", "")

	// Create a new log file for utf encoded messages
	s.Env().VM.Execute("sudo touch /var/log/hello-world-utf.log")
	s.Env().VM.Execute("sudo chmod +r /var/log/hello-world-utf.log")
}

func (s *UtfSuite) TestUtfTailing() {
	// Run test cases
	s.Run("UtfLittleEndianCollection", s.UtfLittleEndianCollection)
	s.Run("UtfBigEndianCollection", s.UtfBigEndianCollection)
}

func (s *UtfSuite) UtfBigEndianCollection() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(
			agentparams.WithLogs(),
			agentparams.WithIntegration("custom_logs.d", string(utfBigEndianLogConfig)))))
	t := s.T()

	// generate utf-16-be log
	content := "big endian sample log"
	err := s.generateUtfLog("be", content)
	assert.NoErrorf(t, err, "Unable to generate utf-16-be log, err: %s.", err)

	// check that intake has utf-16-be encoded log
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().Fakeintake.FilterLogs(service, fi.WithMessageMatching(content))
		assert.NoErrorf(c, err, "Error found: %s", err)
		intakeLogs := logsToString(logs)
		assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', but received %s logs.", content, intakeLogs)
		t.Logf(intakeLogs)
	}, 2*time.Minute, 10*time.Second)

	// flush intake
	s.EventuallyWithT(func(c *assert.CollectT) {
		err := s.Env().Fakeintake.FlushServerAndResetAggregators()
		if assert.NoErrorf(c, err, "Having issues flushing server and resetting aggregators, retrying...") {
			t.Log("Successfully flushed server and reset aggregators.")
		}
	}, 1*time.Minute, 10*time.Second)

}

func (s *UtfSuite) UtfLittleEndianCollection() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(
			agentparams.WithLogs(),
			agentparams.WithIntegration("custom_logs.d", string(utfLittleEndianLogConfig)))))
	t := s.T()

	// generate utf-16-le log
	content := "little endian sample log"
	err := s.generateUtfLog("le", content)
	assert.NoErrorf(t, err, "Unable to generate utf-16-le log, err: %s.", err)

	// check that intake has utf-16-le encoded log
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().Fakeintake.FilterLogs(service, fi.WithMessageMatching(content))
		assert.NoErrorf(c, err, "Error found: %s", err)
		intakeLogs := logsToString(logs)
		assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', but received %s logs.", content, intakeLogs)
		t.Logf(intakeLogs)
	}, 2*time.Minute, 10*time.Second)

	// flush intake
	s.EventuallyWithT(func(c *assert.CollectT) {
		err := s.Env().Fakeintake.FlushServerAndResetAggregators()
		if assert.NoErrorf(c, err, "Having issues flushing server and resetting aggregators, retrying...") {
			t.Log("Successfully flushed server and reset aggregators.")
		}
	}, 1*time.Minute, 10*time.Second)

}
