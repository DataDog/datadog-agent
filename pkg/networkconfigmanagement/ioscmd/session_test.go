// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ioscmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

var errNoMatch = errors.New("no match in buffered output")

// fakeIOS is a synchronous imitation of an IOS shell over a PTY: it echoes
// each line followed by \r\n, writes the line's output, and then writes the
// current prompt.
type fakeIOS struct {
	hostname string
	mode     string // "" for exec mode, or e.g. "config", "config-if"
	userMode bool   // prompt with ">" instead of "#"
	outputs  map[string]string
	stall    string // a line whose output never ends with a prompt

	asked bool // whether the last line asked a question
	buf   []byte
	sent  []string
}

func newFakeIOS(outputs map[string]string) *fakeIOS {
	f := &fakeIOS{hostname: "R1", outputs: outputs}
	f.write(f.prompt())
	return f
}

func (f *fakeIOS) prompt() string {
	p := f.hostname
	if f.mode != "" {
		p += "(" + f.mode + ")"
	}
	if f.userMode {
		return p + ">"
	}
	return p + "#"
}

func (f *fakeIOS) write(s string) {
	f.buf = append(f.buf, strings.ReplaceAll(s, "\n", "\r\n")...)
}

func (f *fakeIOS) Send(_ context.Context, data string) error {
	line := strings.TrimSuffix(data, "\n")
	f.sent = append(f.sent, line)
	f.write(line + "\n")
	asked := f.asked
	f.asked = false
	switch {
	case f.stall != "" && line == f.stall:
		f.write("partial output\n")
		return nil
	case line == "" && asked:
		f.write("Building configuration...\n[OK]\n")
	case line == "copy running-config startup-config":
		f.write("Destination filename [startup-config]? ")
		f.asked = true
		return nil
	case strings.HasPrefix(line, "terminal "), line == "":
	case line == "configure terminal":
		f.write("Enter configuration commands, one per line.  End with CNTL/Z.\n")
		f.mode = "config"
	case line == "end":
		f.mode = ""
	case f.mode != "" && strings.HasPrefix(line, "hostname "):
		f.hostname = strings.TrimPrefix(line, "hostname ")
	case f.mode != "" && strings.HasPrefix(line, "interface "):
		f.mode = "config-if"
	case f.mode != "" && strings.HasPrefix(line, " "):
		// a subcommand; accepted silently
	default:
		out, ok := f.outputs[line]
		if !ok {
			out = "              ^\n% Invalid input detected at '^' marker.\n\n"
		}
		f.write(out)
	}
	f.write(f.prompt())
	return nil
}

func (f *fakeIOS) ReadUntil(_ context.Context, match func([]byte) int) ([]byte, error) {
	i := match(f.buf)
	if i < 0 {
		out := f.buf
		f.buf = nil
		return out, errNoMatch
	}
	out := f.buf[:i]
	f.buf = f.buf[i:]
	return out, nil
}

func TestRun_ShowCommand(t *testing.T) {
	f := newFakeIOS(map[string]string{"show version": "Cisco IOS Software\nUptime 1 day\n"})
	results, err := Run(context.Background(), f, []string{"show version"})
	require.NoError(t, err)
	assert.Equal(t, []*types.CommandResult{
		{CommandStr: "show version", Output: "Cisco IOS Software\nUptime 1 day\n"},
	}, results)
	assert.Equal(t, []string{"terminal length 0", "terminal width 511", "show version"}, f.sent)
	assert.Empty(t, f.buf)
}

func TestRun_MixedConfigAndShow(t *testing.T) {
	f := newFakeIOS(map[string]string{"show ip route": "Gateway of last resort is not set\n"})
	lines := []string{
		"configure terminal",
		"interface Gi0/1",
		" description uplink",
		"end",
		"show ip route",
	}
	results, err := Run(context.Background(), f, lines)
	require.NoError(t, err)
	require.Len(t, results, len(lines))
	for i, line := range lines {
		assert.Equal(t, line, results[i].CommandStr)
		assert.Empty(t, results[i].Error)
	}
	assert.Equal(t, "Enter configuration commands, one per line.  End with CNTL/Z.\n", results[0].Output)
	assert.Equal(t, "", results[1].Output)
	assert.Equal(t, "Gateway of last resort is not set\n", results[4].Output)
}

func TestRun_HostnameChangesPrompt(t *testing.T) {
	f := newFakeIOS(map[string]string{"show clock": "12:00:00\n"})
	results, err := Run(context.Background(), f, []string{"configure terminal", "hostname r2", "end", "show clock"})
	require.NoError(t, err)
	require.Len(t, results, 4)
	assert.Equal(t, "12:00:00\n", results[3].Output)
	assert.Equal(t, "r2", f.hostname)
}

func TestRun_AnswersQuestion(t *testing.T) {
	f := newFakeIOS(nil)
	results, err := Run(context.Background(), f, []string{"copy running-config startup-config", ""})
	require.NoError(t, err)
	assert.Equal(t, []*types.CommandResult{{
		CommandStr: "copy running-config startup-config",
		Output:     "Destination filename [startup-config]? \nBuilding configuration...\n[OK]\n",
	}}, results)
	assert.Empty(t, f.buf)
}

func TestRun_StopsOnErrorAndLeavesConfig(t *testing.T) {
	f := newFakeIOS(nil)
	results, err := Run(context.Background(), f, []string{
		"configure terminal",
		"bogus command",
		"hostname never",
		"end",
	})
	require.ErrorContains(t, err, `running "bogus command": % Invalid input detected`)
	require.Len(t, results, 2)
	assert.Empty(t, results[0].Error)
	assert.Equal(t, "bogus command", results[1].CommandStr)
	assert.Equal(t, "% Invalid input detected at '^' marker.", results[1].Error)
	assert.Contains(t, results[1].Output, "% Invalid input")

	// The run stopped, then sent "end" to leave config mode.
	assert.Equal(t, []string{"terminal length 0", "terminal width 511", "configure terminal", "bogus command", "end"}, f.sent)
	assert.Equal(t, "", f.mode)
	assert.Equal(t, "R1", f.hostname)
}

func TestRun_ErrorOutsideConfigDoesNotSendEnd(t *testing.T) {
	f := newFakeIOS(nil)
	results, err := Run(context.Background(), f, []string{"bogus", "show version"})
	require.Error(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, []string{"terminal length 0", "terminal width 511", "bogus"}, f.sent)
}

func TestRun_RejectsUserExecMode(t *testing.T) {
	f := &fakeIOS{hostname: "R1", userMode: true}
	f.write(f.prompt())
	results, err := Run(context.Background(), f, []string{"show version"})
	assert.ErrorContains(t, err, "not in privileged EXEC mode")
	assert.Nil(t, results)
	assert.Empty(t, f.sent)
}

func TestRun_ReadFailureKeepsPartialOutput(t *testing.T) {
	f := newFakeIOS(map[string]string{"show clock": "12:00:00\n"})
	f.stall = "ping 192.0.2.1"
	results, err := Run(context.Background(), f, []string{"show clock", "ping 192.0.2.1", "show clock"})
	require.ErrorIs(t, err, errNoMatch)
	require.Len(t, results, 2)
	assert.Empty(t, results[0].Error)
	assert.Equal(t, "ping 192.0.2.1", results[1].CommandStr)
	assert.Equal(t, "partial output\n", results[1].Output)
	assert.Equal(t, errNoMatch.Error(), results[1].Error)
}

func TestRun_BannerBeforePrompt(t *testing.T) {
	f := &fakeIOS{hostname: "R1", outputs: map[string]string{"show clock": "12:00:00\n"}}
	f.write("**** Authorized access only ****\n\n" + f.prompt())
	results, err := Run(context.Background(), f, []string{"show clock"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "12:00:00\n", results[0].Output)
}

func TestIOSError(t *testing.T) {
	errorLines := []string{
		"% Invalid input detected at '^' marker.",
		"% Invalid IP address",
		"% Invalid interface type and number",
		"% Incomplete command.",
		`% Ambiguous command:  "sh i"`,
		"% Unrecognized command",
		"% Unknown command or computer name, or unable to find computer address",
		"% Unknown VRF specified",
		"% Bad IP address or host name",
		"% Bad mask 0xFF000000 for address 10.0.0.1",
		"%Error opening flash:foo (No such file or directory)",
		"% Error in authentication.",
		"% Cannot remove the last IP address",
		"% Can't find interface",
		"% Unable to open file",
		"% Failed to write configuration",
		"% No such file or directory",
		"% Not enough space on device",
		"% Insufficient memory",
		"% 10.0.0.0 overlaps with GigabitEthernet0/1",
		"% Command authorization failed.",
		"% Authorization failed.",
		"% Access denied",
		"% Permission denied",
	}
	for _, line := range errorLines {
		t.Run(line, func(t *testing.T) {
			output := "R1(config)#some command\n" + line + "\n\n"
			err := iosError(output)
			require.Error(t, err)
			assert.Equal(t, line, err.Error())
		})
	}

	okOutputs := []string{
		"",
		"Building configuration...\n[OK]\n",
		"% Warning: use /31 mask on non point-to-point interface cautiously\n",
		"% Note: this command is deprecated\n",
		"%SYS-5-CONFIG_I: Configured from console by console\n",
		"description % Invalid input\n", // not at line start
	}
	for _, output := range okOutputs {
		t.Run("ok/"+output, func(t *testing.T) {
			assert.NoError(t, iosError(output))
		})
	}
}
