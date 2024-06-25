// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package linuxfiletailing

import (
	_ "embed"
	"fmt"
	"testing"
	"time"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

//go:embed config/utf-16-le.yaml
var utfLittleEndianLogConfig []byte

//go:embed config/utf-16-be.yaml
var utfBigEndianLogConfig []byte

const utfservice = "utfservice"

// UTFSuite defines a test suite for the log agent tailing UTF encoded logs
type UtfSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestUtfSuite runs the E2E test suite for tailing UTF encoded logs
func TestUtfSuite(t *testing.T) {
	s := &UtfSuite{}
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithLogs(), agentparams.WithIntegration("custom_logs.d", logConfig)))),
	}

	e2e.Run(t, s, options...)
}

func (s *UtfSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	// flush intake
	s.EventuallyWithT(func(c *assert.CollectT) {
		err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		if assert.NoErrorf(c, err, "Having issues flushing server and resetting aggregators, retrying...") {
			s.T().Log("Successfully flushed server and reset aggregators.")
		}
	}, 1*time.Minute, 10*time.Second)

	// Create a new log file for utf encoded messages
	s.Env().RemoteHost.MustExecute("sudo touch /var/log/hello-world-utf.log")
	s.Env().RemoteHost.MustExecute("sudo chmod +r /var/log/hello-world-utf.log")
}

func (s *UtfSuite) AfterTest(suiteName, testName string) {
	s.BaseSuite.AfterTest(suiteName, testName)

	// delete log file
	s.Env().RemoteHost.MustExecute("sudo rm /var/log/hello-world-utf.log")
}

func (s *UtfSuite) TestUtfTailing() {
	// Run test cases
	s.Run("UtfLittleEndianCollection", s.testUtfLittleEndianCollection)
	s.Run("UtfBigEndianCollection", s.testUtfBigEndianCollection)
}

func (s *UtfSuite) testUtfBigEndianCollection() {
	agentOptions := []agentparams.Option{
		agentparams.WithLogs(),
		agentparams.WithIntegration("custom_logs.d", string(utfBigEndianLogConfig)),
	}
	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentOptions...)))

	// generate utf-16-be log
	content := "big endian sample log"
	s.generateUtfLog("be", content)

	// check that intake has utf-16-be encoded log
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(utfservice, fi.WithMessageMatching(content))
		assert.NoErrorf(c, err, "Error found: %s", err)
		assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', from service: %s.", content, utfservice)
	}, 2*time.Minute, 10*time.Second)
}

func (s *UtfSuite) testUtfLittleEndianCollection() {
	agentOptions := []agentparams.Option{
		agentparams.WithLogs(),
		agentparams.WithIntegration("custom_logs.d", string(utfLittleEndianLogConfig)),
	}
	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentOptions...)))

	// generate utf-16-le log
	content := "little endian sample log"
	s.generateUtfLog("le", content)

	// check that intake has utf-16-le encoded log
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(utfservice, fi.WithMessageMatching(content))
		assert.NoErrorf(c, err, "Error found: %s", err)
		assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', from service: %s.", content, utfservice)
	}, 2*time.Minute, 10*time.Second)
}

func (s *UtfSuite) generateUtfLog(endianness, content string) {
	s.T().Helper()
	utfLogGenerationCommand := fmt.Sprintf(`sudo python3 -c "f = open('/var/log/hello-world-utf.log', 'ab'); t = '%s\n'.encode('utf-16-%s'); f.write(t); f.close()"`, content, endianness)
	s.Env().RemoteHost.MustExecute(utfLogGenerationCommand)
}
