// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package linuxfiletailing

import (
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/utils"
)

const (
	ottlLogFileName = "ottl.json"
	ottlLogFilePath = utils.LinuxLogsFolderPath + "/" + ottlLogFileName
	ottlService     = "ottl-filtering"
)

// TestOTTLFiltering exercises configuration, file tailing, filtering, and encoding
// through the running Agent. The final retained record is a flush control for
// the dropped records written before it in the same stream.
func (s *LinuxFakeintakeSuite) TestOTTLFiltering() {
	t := s.T()
	s.Env().RemoteHost.MustExecute("sudo touch " + ottlLogFilePath)
	s.Env().RemoteHost.MustExecute("sudo chmod 666 " + ottlLogFilePath)
	t.Cleanup(func() {
		_, err := s.Env().RemoteHost.Execute("sudo rm -f " + ottlLogFilePath)
		require.NoError(t, err, "Unable to remove the OTTL log fixture")
	})
	utils.AssertAgentTailerOK(s, ottlLogFileName)

	dropped := []string{
		`{"env":"dev","message":"drop-source-include"}`,
		`{"env":"prod","http":{"path":"/health","status":200},"message":"drop-source-exclude"}`,
		`{"env":"prod","ottl_global_drop":true,"http":{"path":"/checkout","status":503},"message":"drop-global"}`,
	}
	malformed := `ottl-invalid-json`
	retained := `{"env":"prod","http":{"path":"/checkout","status":503},"message":"ottl-kept"}`
	records := append(append([]string{}, dropped...), malformed, retained)
	utils.AppendLog(s, ottlLogFileName, strings.Join(records, "\n"), 1)

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(ottlService)
		require.NoError(c, err, "Unable to query OTTL logs in fakeintake")
		messages := make([]string, 0, len(logs))
		for _, log := range logs {
			messages = append(messages, log.Message)
		}
		require.Contains(c, messages, retained, "the retained JSON must reach intake unchanged")
		require.Contains(c, messages, malformed, "evaluation errors must keep the message")
		for _, record := range dropped {
			assert.NotContains(c, messages, record, "OTTL-filtered logs must not reach intake")
		}
	}, 2*time.Minute, 10*time.Second)
}
