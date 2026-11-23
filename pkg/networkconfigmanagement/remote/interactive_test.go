// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remote

import (
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

const fakePanOSPrompt = "admin@PA-440(passive)> "

// fakePanOS mimics a PAN-OS CLI on a PTY: a login banner, then for each line
// it echoes the line, prints the canned output (with CRLF line endings), and
// shows the prompt again. Unknown lines get "Invalid syntax.". A nil output
// map value makes the device print nothing more, never showing the prompt.
func fakePanOS(banner string, outputs map[string]*string) SessionFactory {
	return func(sh *ShellContext) ShellFunc {
		crlf := func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }
		_, _ = io.WriteString(sh.stdout, crlf(banner)+fakePanOSPrompt)
		return func(command string, sh *ShellContext) uint32 {
			if command == "" || command == "exit" {
				return 0
			}
			_, _ = io.WriteString(sh.stdout, command+"\r\n")
			out, ok := outputs[command]
			if !ok {
				_, _ = io.WriteString(sh.stdout, "Invalid syntax.\r\n\r\n"+fakePanOSPrompt)
				return 0
			}
			if out == nil {
				return 0
			}
			_, _ = io.WriteString(sh.stdout, crlf(*out)+"\r\n"+fakePanOSPrompt)
			return 0
		}
	}
}

func ptr(s string) *string { return &s }

func panOSCommand(command, expect string) *profile.PlainCommand {
	return profile.MkCommand(command,
		profile.Expect(expect),
		profile.WithSetup("set cli pager off"),
		profile.WithPrompt(`(?:^|[\r\n])[\w.\-]+@[\w.\-]+(?:\([\w\-]+\))?[>#] ?$`),
	)
}

const panOSBanner = "\n\n\nNumber of failed attempts since last successful login: 0\n\n\n\n"

func TestExecuteInteractive(t *testing.T) {
	xmlConfig := "<config version=\"11.1.0\">\n  <mgt-config/>\n</config>"
	curlyConfig := "config {\n  mgt-config {\n  }\n}"
	tests := []struct {
		name    string
		outputs map[string]*string
		want    string
	}{
		{"xml", map[string]*string{"set cli pager off": ptr(""), "show config running": ptr(xmlConfig)}, xmlConfig},
		{"curly", map[string]*string{"set cli pager off": ptr(""), "show config running": ptr(curlyConfig)}, curlyConfig},
		// A failing setup command is logged, and the command still runs.
		{"setup fails", map[string]*string{"show config running": ptr(xmlConfig)}, xmlConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := StartFakeSSHServerWithFactory(t, fakePanOS(panOSBanner, tt.outputs))
			conn := connectTestConn(t, srv)
			cmd := panOSCommand("show config running", `(?s)<config.*</config>|(?m)^config \{`)

			res, err := ExecuteInteractive(t.Context(), conn.StartShell, cmd)
			require.NoError(t, err)
			assert.Equal(t, tt.want, res.Output)
			assert.Equal(t, "show config running", res.CommandStr)
			assert.Equal(t, []string{"set cli pager off", "show config running"}, srv.Received()[:2])
		})
	}
}

func TestExecuteInteractive_ValidationFails(t *testing.T) {
	srv := StartFakeSSHServerWithFactory(t, fakePanOS(panOSBanner, map[string]*string{"set cli pager off": ptr("")}))
	conn := connectTestConn(t, srv)

	res, err := ExecuteInteractive(t.Context(), conn.StartShell, panOSCommand("show config running", `<config`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match required regex")
	assert.Equal(t, "Invalid syntax.", res.Output)
}

func TestExecuteInteractive_NoPrompt(t *testing.T) {
	setTimeouts(t, 200*time.Millisecond, time.Second)
	// A device that never shows a PAN-OS prompt, e.g. while testing the
	// pan-os profile against a different vendor.
	srv := StartFakeSSHServerWithFactory(t, func(sh *ShellContext) ShellFunc {
		_, _ = io.WriteString(sh.stdout, "Welcome\r\nrouter#")
		return func(string, *ShellContext) uint32 { return 0 }
	})
	conn := connectTestConn(t, srv)

	start := time.Now()
	_, err := ExecuteInteractive(t.Context(), conn.StartShell, panOSCommand("show system info", `model`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "waiting for the device prompt")
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Empty(t, srv.Received())
}

func TestExecuteInteractive_CommandTimesOut(t *testing.T) {
	setTimeouts(t, time.Second, 200*time.Millisecond)
	srv := StartFakeSSHServerWithFactory(t, fakePanOS(panOSBanner, map[string]*string{
		"set cli pager off":   ptr(""),
		"show config running": nil,
	}))
	conn := connectTestConn(t, srv)

	_, err := ExecuteInteractive(t.Context(), conn.StartShell, panOSCommand("show config running", `<config`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `waiting for the prompt after "show config running"`)
}

func TestExecuteInteractive_LongOutput(t *testing.T) {
	// Output much larger than the prompt search window, as a real config is.
	config := "<config>\n" + strings.Repeat("  <entry name=\"x\">y</entry>\n", 20000) + "</config>"
	srv := StartFakeSSHServerWithFactory(t, fakePanOS(panOSBanner, map[string]*string{
		"set cli pager off":   ptr(""),
		"show config running": ptr(config),
	}))
	conn := connectTestConn(t, srv)

	res, err := ExecuteInteractive(t.Context(), conn.StartShell, panOSCommand("show config running", `(?s)<config.*</config>`))
	require.NoError(t, err)
	assert.Equal(t, config, res.Output)
}

func TestSSHConnection_InteractiveProfile(t *testing.T) {
	srv := StartFakeSSHServerWithFactory(t, fakePanOS(panOSBanner, map[string]*string{
		"set cli pager off":   ptr(""),
		"show system info":    ptr("hostname: PA-440\nmodel: PA-440"),
		"show config running": ptr("config {\n}"),
	}))
	conn := connectTestConn(t, srv)
	prof, err := profile.GetProfileMap()
	require.NoError(t, err)
	conn.SetProfile(prof[profile.ProfilePanOS])

	require.NoError(t, conn.Verify(t.Context()))
	res, err := conn.RetrieveRunningConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "config {\n}", res.Output)
}

func TestCleanOutput(t *testing.T) {
	prompt := regexp.MustCompile(`(?:^|[\r\n])admin@fw> ?$`)
	raw := "show config running\r\n\x1b[1mconfig {\r\n}\x1b[0m\r\n\r\nadmin@fw> "
	assert.Equal(t, "config {\n}", string(cleanOutput([]byte(raw), "show config running", prompt)))
	// No echo to strip.
	assert.Equal(t, "config {\n}", string(cleanOutput([]byte("config {\r\n}\r\nadmin@fw> "), "show config running", prompt)))
}

func setTimeouts(t *testing.T, login, command time.Duration) {
	oldLogin, oldCommand := interactiveLoginTimeout, interactiveCommandTimeout
	interactiveLoginTimeout, interactiveCommandTimeout = login, command
	t.Cleanup(func() { interactiveLoginTimeout, interactiveCommandTimeout = oldLogin, oldCommand })
}
