// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
)

type authArtifactWindows struct {
	authArtifactBase
}

func TestIPCSecurityWindowsSuite(t *testing.T) {
	t.Parallel()

	e2e.Run(t,
		&authArtifactWindows{
			authArtifactBase{
				svcName:            "datadogagent",
				authTokenPath:      `C:\ProgramData\Datadog\auth_token`,
				ipcCertPath:        `C:\ProgramData\Datadog\ipc_cert.pem`,
				removeFilesCmdTmpl: "powershell -Command \"Remove-Item -Path %s\\* -Recurse; Remove-Item -Path %s; Remove-Item -Path %s\"",
				readLogCmdTmpl:     "powershell -Command \"Get-Content -Path %v -Wait\"",
				pathJoinFunction:   join,
				agentProcesses:     []string{"agent", "trace-agent", "process-agent"},
			},
		},
		e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithName("authArtifactWindows"),
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)),
				ec2.WithAgentClientOptions(agentclientparams.WithSkipWaitForAgentReady()),
			),
		)),
		e2e.WithSkipCoverage(), // Test Suite is not compatible with coverage computation, because auth tokens are removed at the end of the test
	)
}

// Implementation of [path.Join] for Windows.
// It was copied from the Go source code.
//
// [path.Join]: https://cs.opensource.google/go/go/+/refs/tags/go1.23.6:src/path/filepath/path_windows.go;l=65-110
func join(elem ...string) string {
	var b strings.Builder
	var lastChar byte
	for _, e := range elem {
		switch {
		case b.Len() == 0:
			// Add the first non-empty path element unchanged.
		case os.IsPathSeparator(lastChar):
			// If the path ends in a slash, strip any leading slashes from the next
			// path element to avoid creating a UNC path (any path starting with "\\")
			// from non-UNC elements.
			//
			// The correct behavior for Join when the first element is an incomplete UNC
			// path (for example, "\\") is underspecified. We currently join subsequent
			// elements so Join("\\", "host", "share") produces "\\host\share".
			for len(e) > 0 && os.IsPathSeparator(e[0]) {
				e = e[1:]
			}
			// If the path is \ and the next path element is ??,
			// add an extra .\ to create \.\?? rather than \??\
			// (a Root Local Device path).
			if b.Len() == 1 && strings.HasPrefix(e, "??") && (len(e) == len("??") || os.IsPathSeparator(e[2])) {
				b.WriteString(`.\`)
			}
		case lastChar == ':':
			// If the path ends in a colon, keep the path relative to the current directory
			// on a drive and don't add a separator. Preserve leading slashes in the next
			// path element, which may make the path absolute.
			//
			// 	Join(`C:`, `f`) = `C:f`
			//	Join(`C:`, `\f`) = `C:\f`
		default:
			// In all other cases, add a separator between elements.
			b.WriteByte('\\')
			lastChar = '\\'
		}
		if len(e) > 0 {
			b.WriteString(e)
			lastChar = e[len(e)-1]
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return filepath.Clean(b.String())
}
