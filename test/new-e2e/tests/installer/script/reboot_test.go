// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installscript

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// rebootAndWait reboots the host and blocks until it comes back up with a fresh
// boot id, reconnecting the SSH client. It is generic across installer-script
// suites that need to assert behavior across a reboot.
func rebootAndWait(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	before := strings.TrimSpace(host.MustExecute("cat /proc/sys/kernel/random/boot_id"))
	// The SSH connection drops as the host goes down; ignore the error.
	_, _ = host.Execute("sudo reboot")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		if err := host.Reconnect(); err != nil {
			assert.NoError(c, err)
			return
		}
		out, err := host.Execute("cat /proc/sys/kernel/random/boot_id")
		if !assert.NoError(c, err) {
			return
		}
		assert.NotEqualf(c, before, strings.TrimSpace(out), "host has not rebooted yet")
	}, 5*time.Minute, 10*time.Second)
}
