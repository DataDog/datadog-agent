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

// TestOTTLFilteringAndMasking exercises configuration, file tailing, filtering, masking, and encoding
// through the running Agent. The final retained record is a flush control for
// the dropped records written before it in the same stream.
func (s *LinuxFakeintakeSuite) TestOTTLFilteringAndMasking() {
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
		`{"env":"prod","http":{"path":"/checkout","status":503},"credentials":true,"message":"drop-invalid-mask-target"}`,
		`{"env":"prod","credentials":`,
	}
	malformed := `ottl-invalid-json`
	retained := `{"env":"prod","http":{"path":"/checkout","status":503},"token":"secret-global","credentials":{"password":"secret-source"},"id":9007199254740993,"message":"ottl-kept"}`
	masked := `{"env":"prod","http":{"path":"/checkout","status":503},"token":"***","credentials":{"password":"***"},"id":9007199254740993,"message":"ottl-kept"}`
	raw := "ottl-raw-raw-secret"
	records := append(append([]string{}, dropped...), malformed, raw, retained)
	utils.AppendLog(s, ottlLogFileName, strings.Join(records, "\n"), 1)

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(ottlService)
		require.NoError(c, err, "Unable to query OTTL logs in fakeintake")
		messages := make([]string, 0, len(logs))
		for _, log := range logs {
			messages = append(messages, log.Message)
		}
		require.Contains(c, messages, masked, "global and source masking must redact fields without changing integer IDs")
		require.Contains(c, messages, "ottl-raw-***", "body masking must work on non-JSON messages")
		assert.NotContains(c, messages, retained, "unredacted JSON must not reach intake")
		assert.NotContains(c, messages, raw, "unredacted body must not reach intake")
		require.Contains(c, messages, malformed, "rules with false conditions must leave plain text untouched")
		for _, record := range dropped {
			assert.NotContains(c, messages, record, "OTTL-filtered logs must not reach intake")
		}
	}, 2*time.Minute, 10*time.Second)
}
