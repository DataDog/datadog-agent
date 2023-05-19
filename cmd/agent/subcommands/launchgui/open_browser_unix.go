// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build freebsd || netbsd || openbsd || solaris || dragonfly || linux

package launchgui

// opens a browser window at the specified URL
import "os/exec"

func open(url string) error {
	return exec.Command("xdg-open", url).Start()
}
