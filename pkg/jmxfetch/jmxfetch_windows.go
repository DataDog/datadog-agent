// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build jmx

package jmxfetch

func (j *JMXFetch) Monitor() {}

// Stop stops the JMXFetch process
func (j *JMXFetch) Stop() error {
	return j.cmd.Process.Kill()
}
