// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procutil

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func TestCommandLineSplitting(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected []string
	}{
		{
			input: "\"C:\\Users\\db\\AppData\\Local\\slack\app-3.1.1\\slack.exe\" --type=gpu-process --no-sandbox --supports-dual-gpus=false --gpu-driver-bug-workarounds=7,10,20,21,24,43,76 --disable-gl-extensions=\"GL_KHR_blend_equation_advanced GL_KHR_blend_equation_advanced_coherent\" --gpu-vendor-id=0x10de --gpu-device-id=0x13b2 --gpu-driver-vendor=NVIDIA --gpu-driver-version=22.21.13.8205 --gpu-driver-date=5-1-2017 --gpu-secondary-vendor-ids=0x8086 --gpu-secondary-device-ids=0x191b --service-request-channel-token=2EADF7A9FD7CB01C6A780DE1F8FEF0BB --mojo-platform-channel-handle=1708 /prefetch:2",
			expected: []string{
				"\"C:\\Users\\db\\AppData\\Local\\slack\app-3.1.1\\slack.exe\"",
				"--type=gpu-process",
				"--no-sandbox",
				"--supports-dual-gpus=false",
				"--gpu-driver-bug-workarounds=7,10,20,21,24,43,76",
				"--disable-gl-extensions=\"GL_KHR_blend_equation_advanced GL_KHR_blend_equation_advanced_coherent\"",
				"--gpu-vendor-id=0x10de",
				"--gpu-device-id=0x13b2",
				"--gpu-driver-vendor=NVIDIA",
				"--gpu-driver-version=22.21.13.8205",
				"--gpu-driver-date=5-1-2017",
				"--gpu-secondary-vendor-ids=0x8086",
				"--gpu-secondary-device-ids=0x191b",
				"--service-request-channel-token=2EADF7A9FD7CB01C6A780DE1F8FEF0BB",
				"--mojo-platform-channel-handle=1708",
				"/prefetch:2",
			},
		},
		{
			input: "\"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe\" --type=renderer --field-trial-handle=1592,5674313428440474125,10112982115004747190,131072 --service-pipe-token=E553C13F2DAFB1BDFD9B6F4F2B98B2ED --lang=en-US --enable-offline-auto-reload --enable-offline-auto-reload-visible-only --device-scale-factor=1 --num-raster-threads=4 --enable-main-frame-before-activation --enable-compositor-image-animations --service-request-channel-token=E553C13F2DAFB1BDFD9B6F4F2B98B2ED --renderer-client-id=1103 --mojo-platform-channel-handle=13292 /prefetch:1",
			expected: []string{"\"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe\"",
				"--type=renderer",
				"--field-trial-handle=1592,5674313428440474125,10112982115004747190,131072",
				"--service-pipe-token=E553C13F2DAFB1BDFD9B6F4F2B98B2ED",
				"--lang=en-US",
				"--enable-offline-auto-reload",
				"--enable-offline-auto-reload-visible-only",
				"--device-scale-factor=1",
				"--num-raster-threads=4",
				"--enable-main-frame-before-activation",
				"--enable-compositor-image-animations",
				"--service-request-channel-token=E553C13F2DAFB1BDFD9B6F4F2B98B2ED",
				"--renderer-client-id=1103",
				"--mojo-platform-channel-handle=13292",
				"/prefetch:1",
			},
		},
	} {
		assert.Equal(t, tc.expected, ParseCmdLineArgs(tc.input))
	}
}

func TestWindowsStringConversion(t *testing.T) {
	for _, tc := range []struct {
		input    []uint16
		expected string
	}{
		{
			input: []uint16{
				0x54, 0x68, 0x69, 0x73, 0x20, 0x69, 0x73, 0x20, 0x61, 0x20, 0x74, 0x65, 0x73, 0x74, 0x20, 0x73, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x0},
			expected: "This is a test string",
		},
		{
			input: []uint16{
				0x2e, 0x4e, 0x45, 0x54, 0x20, 0x43, 0x4c, 0x52, 0x2d, 0x73, 0xe4, 0x6b, 0x65, 0x72, 0x68, 0x65, 0x74, 0x20, 0x21, 0x20, 0x4d, 0x69, 0x63, 0x72, 0x6f, 0x73, 0x6f, 0x66, 0x74, 0x2e, 0x45, 0x78, 0x63, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x2e, 0x55, 0x4d, 0x2e, 0x43, 0x61, 0x6c, 0x6c, 0x52, 0x6f, 0x75, 0x74, 0x65, 0x72, 0x20, 0x21, 0x20, 0x54, 0x69, 0x64, 0x20, 0x66, 0xf6, 0x72, 0x20, 0x6b, 0xf6, 0x72, 0x6e, 0x69, 0x6e, 0x67, 0x73, 0x6b, 0x6f, 0x6e, 0x74, 0x72, 0x6f, 0x6c, 0x6c, 0x65, 0x72, 0x20, 0x69, 0x20, 0x70, 0x72, 0x6f, 0x63, 0x65, 0x6e, 0x74, 0x0},
			expected: ".NET CLR-säkerhet ! Microsoft.Exchange.UM.CallRouter ! Tid för körningskontroller i procent",
		},
	} {
		assert.Equal(t, tc.expected, winutil.ConvertWindowsString16(tc.input))
	}
}

func TestWindowsProbe(t *testing.T) {
	tests := map[string]Probe{
		"toolhelp":     NewWindowsToolhelpProbe(),
		"perfcounters": NewProcessProbe(),
	}

	for name, probe := range tests {
		t.Run(name, func(t *testing.T) {
			now := time.Now()
			cmd := exec.Command("powershell.exe", "-c", "sleep 10; foo bar baz")

			err := cmd.Start()
			assert.NoError(t, err)

			defer cmd.Process.Kill()

			procs, err := probe.ProcessesByPID(time.Now(), true)

			assert.NoError(t, err)
			p, found := procs[int32(cmd.Process.Pid)]

			assert.True(t, found)
			assert.True(t, strings.HasSuffix(p.Exe, "powershell.exe"))
			assert.Equal(t, []string{"powershell.exe", "-c", `"sleep 10; foo bar baz"`}, p.Cmdline)
			assert.Equal(t, int32(os.Getpid()), p.Ppid)
			assert.Equal(t, int32(cmd.Process.Pid), p.Pid)

			assert.WithinRange(t, time.Unix(0, p.Stats.CreateTime*1000_000), now, now.Add(5*time.Second))

			stats, err := probe.StatsForPIDs([]int32{p.Pid}, time.Now())
			assert.NoError(t, err)

			// Make no assumption about the values of stats - these are going to change as the process runs and
			// can vary wildly depending on the env
			assert.Equal(t, p.Stats.CreateTime, stats[p.Pid].CreateTime)
		})
	}
}
