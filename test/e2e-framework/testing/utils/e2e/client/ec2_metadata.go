// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// EC2Metadata contains a pointer to a VM and its AWS token
type EC2Metadata struct {
	t        *testing.T
	host     *Host
	osFamily os.Family
	token    string
}

const metadataEndPoint = "http://169.254.169.254"
const commandTimeoutSec = 5

// NewEC2Metadata creates a new [EC2Metadata] given an EC2 [VM]
func NewEC2Metadata(t *testing.T, h *Host, osFamily os.Family) *EC2Metadata {
	if osFamily != os.WindowsFamily && osFamily != os.LinuxFamily {
		panic(fmt.Sprintf("unsupported OS family: %v", osFamily))
	}

	m := &EC2Metadata{osFamily: osFamily, host: h, t: t}
	t.Log("Getting EC2 metadata token")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.NoError(c, m.refreshToken())
	}, time.Second*30, time.Second*1)
	return m
}

// Get returns EC2 instance name
func (m *EC2Metadata) Get(name string) string {
	m.t.Log("Getting EC2 metadata for", name)
	var output string
	require.EventuallyWithT(m.t, func(c *assert.CollectT) {
		if m.token == "" {
			require.NoError(c, m.refreshToken())
		}

		var cmd string
		switch m.osFamily {
		case os.WindowsFamily:
			cmd = fmt.Sprintf(`Invoke-RestMethod  -Headers @{"X-aws-ec2-metadata-token"="%v"} -Uri "%v/latest/meta-data/%v" -TimeoutSec %v`, m.token, metadataEndPoint, name, commandTimeoutSec)
		case os.LinuxFamily:
			cmd = fmt.Sprintf(`curl -sS --fail -H "X-aws-ec2-metadata-token: %v" "%v/latest/meta-data/%v" --max-time %v`, m.token, metadataEndPoint, name, commandTimeoutSec)
		}

		var err error
		output, err = m.host.Execute(cmd)
		if err != nil {
			// The token may have expired or been revoked, get a new one on the next attempt
			m.token = ""
		}
		require.NoError(c, err)
	}, time.Second*30, time.Second*1)
	return strings.TrimRight(output, "\r\n")
}

// refreshToken requests a new IMDSv2 session token and stores it on success
func (m *EC2Metadata) refreshToken() error {
	var cmd string
	switch m.osFamily {
	case os.WindowsFamily:
		cmd = fmt.Sprintf(`Invoke-RestMethod -Uri "%v/latest/api/token" -Method Put -Headers @{ "X-aws-ec2-metadata-token-ttl-seconds" = "21600" } -TimeoutSec %v`, metadataEndPoint, commandTimeoutSec)
	case os.LinuxFamily:
		cmd = fmt.Sprintf(`curl -sS --fail -X PUT "%v/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600" --max-time %v`, metadataEndPoint, commandTimeoutSec)
	}

	token, err := m.host.Execute(cmd)
	if err != nil {
		return err
	}
	m.token = token
	return nil
}
