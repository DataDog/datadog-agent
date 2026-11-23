// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const procmgrServiceName = "dd-procmgr-service"

func TestProcmgrLocalSystem(t *testing.T) {
	s := &procmgrLocalSystemSuite{}
	Run(t, s)
}

type procmgrLocalSystemSuite struct {
	baseAgentMSISuite
}

func (s *procmgrLocalSystemSuite) TestProcmgrServiceRunsAsLocalSystem() {
	host := s.Env().RemoteHost
	_ = s.installAgentPackage(host, s.AgentPackage)

	account, err := windowsCommon.GetServiceAccountName(host, procmgrServiceName)
	s.Require().NoError(err)
	s.Assert().Equal("LocalSystem", account, "%s SCM account", procmgrServiceName)

	s.Require().NoError(windowsCommon.StartService(host, procmgrServiceName))

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		owner, err := windowsProcessOwnerByName(host, "dd-procmgrd.exe")
		assert.NoError(ct, err)
		assert.Contains(ct, owner, "NT AUTHORITY/SYSTEM")
	}, 60*time.Second, 2*time.Second)
}

func windowsProcessOwnerByName(host *components.RemoteHost, name string) (string, error) {
	script := fmt.Sprintf(
		`$p = Get-CimInstance Win32_Process -Filter "Name='%s'" | Select-Object -First 1; if ($null -eq $p) { exit 1 }; $o = Invoke-CimMethod -InputObject $p -MethodName GetOwner; if ($o.ReturnValue -ne 0) { exit $o.ReturnValue }; "$($o.Domain)/$($o.User)"`,
		name,
	)
	out, err := host.Execute(script)
	return strings.TrimSpace(out), err
}
