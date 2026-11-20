// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package darwinunits

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
)

// TestLaunchdJobsArePropertyLists ensures every job is actually compiled in, and that the
// definitions are property lists rather than whatever the template happened to emit.
func TestLaunchdJobsArePropertyLists(t *testing.T) {
	for _, job := range units.Fields(Launchd) {
		label := job.Name
		t.Run(label, func(t *testing.T) {
			content, err := Content(job)
			require.NoError(t, err)
			assert.NotEmpty(t, content)

			assertPropertyList(t, content)
			// RunAtLoad is what starts the job at boot with no login session, so it is
			// required on every definition rather than merely typical of them.
			assert.Regexp(t, `<key>RunAtLoad</key>\s*<true/>`, string(content))
			// Every job is supervised: launchd relaunches a job that exits unsuccessfully.
			assert.Contains(t, string(content), "KeepAlive")
			assert.Contains(t, string(content), fmt.Sprintf("<string>%s</string>", label))
		})
	}
}

func TestLaunchdAgentAndSysprobeReadTheSameConfig(t *testing.T) {
	const etcDir = "/opt/datadog-agent/etc"
	for _, job := range []units.Unit{Launchd.Agent, Launchd.Sysprobe} {
		label := job.Name
		content, err := Content(job)
		require.NoError(t, err)
		var plist struct {
			Args []string `xml:"dict>array>string"`
		}
		require.NoError(t, xml.Unmarshal(content, &plist))
		require.GreaterOrEqual(t, len(plist.Args), 4)
		assert.Equal(t, []string{"run", "-c", etcDir}, plist.Args[1:4])
		if label == "com.datadoghq.agent" {
			require.GreaterOrEqual(t, len(plist.Args), 5)
			assert.Equal(t, "--sysprobecfgpath="+etcDir+"/system-probe.yaml", plist.Args[4],
				"the Agent loads system-probe config separately from its main config")
			// No pidfile: nothing reads it, and the Agent refuses to start when a stale one
			// names a PID that another process has since reused, which launchd's KeepAlive
			// would turn into a restart loop.
			assert.Len(t, plist.Args, 5)
			assert.NotContains(t, plist.Args, "-p")
		}
	}
}

// TestLaunchdInstallerJob pins the root installer daemon's job: it runs as root, logs next to the
// Agent's other jobs, leaves run_path to the configuration and writes its pidfile in the Agent's run
// directory, as on Linux.
func TestLaunchdInstallerJob(t *testing.T) {
	content, err := Content(Launchd.Installer)
	require.NoError(t, err)
	definition := string(content)

	assert.NotContains(t, definition, "DD_RUN_PATH")
	assert.Regexp(t, `<key>StandardOutPath</key>\s*<string>/opt/datadog-agent/logs/launchd_installer.log</string>`, definition)
	assert.Regexp(t, `<key>StandardErrorPath</key>\s*<string>/opt/datadog-agent/logs/launchd_installer.log</string>`, definition)
	assert.Regexp(t, `<key>UserName</key>\s*<string>root</string>`, definition)

	var plist struct {
		Args []string `xml:"dict>array>string"`
	}
	require.NoError(t, xml.Unmarshal(content, &plist))
	assert.Equal(t, []string{"/opt/datadog-agent/embedded/bin/installer", "run", "-c", "/opt/datadog-agent/etc", "-p", "/opt/datadog-agent/run/installer.pid"}, plist.Args)
}

// assertPropertyList checks that content is a well-formed XML property list. The repository has no
// plist decoder, and pulling one in for a structural check would be a dependency for one
// assertion; launchd will refuse a definition that is not well-formed XML rooted at <plist>, which
// is what this catches.
func assertPropertyList(t *testing.T, content []byte) {
	t.Helper()

	decoder := xml.NewDecoder(bytes.NewReader(content))
	var root string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "definition is not well-formed XML")
		if start, ok := token.(xml.StartElement); ok && root == "" {
			root = start.Name.Local
		}
	}
	assert.Equal(t, "plist", root, "definition is not rooted at <plist>")
}

func TestProcmgrProcessesAreInert(t *testing.T) {
	for _, process := range units.Fields(Procmgr) {
		t.Run(process.Name, func(t *testing.T) {
			content, err := Content(process)
			require.NoError(t, err)
			require.NotEmpty(t, content)
			text := string(content)
			assert.Contains(t, text, "auto_start: false")
			assert.Contains(t, text, "/opt/datadog-agent/embedded/bin/")
			assert.NotContains(t, text, "{{", "a template action was left unrendered")
			assert.NotContains(t, text, "__", "an install-time placeholder would never be replaced on macOS")
			assert.NotContains(t, text, ".exe")
			assert.NotRegexp(t, `[A-Za-z]:\\`, text, "a Windows drive path")
		})
	}
}
