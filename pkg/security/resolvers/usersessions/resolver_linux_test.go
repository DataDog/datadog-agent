// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package usersessions holds model related to the user sessions resolver
package usersessions

import (
	"testing"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model/usersession"
)

func Test_parseSSHLogLine(t *testing.T) {
	tests := []struct {
		name               string
		logLine            string
		expectedIP         string
		expectedPort       string
		expectedAuthMethod int
		expectedPublicKey  string
		expectedSSHDPid    string
		shouldAddToLRU     bool
	}{
		{
			name:               "ISO 8601 timestamp format with sshd",
			logLine:            "2025-11-05T10:00:25.279861+01:00 lima-dev sshd[1234]: Accepted publickey for lima from 192.168.5.2 port 38835 ssh2: ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU",
			expectedIP:         "192.168.5.2",
			expectedPort:       "38835",
			expectedAuthMethod: int(usersession.SSHAuthMethodPublicKey),
			expectedPublicKey:  "J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU",
			expectedSSHDPid:    "1234",
			shouldAddToLRU:     true,
		},
		{
			name:               "Traditional syslog timestamp format with sshd",
			logLine:            "Nov  5 09:59:44 lima-centos7-9-x64 sshd[5678]: Accepted publickey for lima from 192.168.5.2 port 38835 ssh2: ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU",
			expectedIP:         "192.168.5.2",
			expectedPort:       "38835",
			expectedAuthMethod: int(usersession.SSHAuthMethodPublicKey),
			expectedPublicKey:  "J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU",
			expectedSSHDPid:    "5678",
			shouldAddToLRU:     true,
		},
		{
			name:               "Password authentication",
			logLine:            "Nov  5 09:59:44 lima-centos7-9-x64 sshd[5678]: Accepted password for testuser from 10.0.0.1 port 12345 ssh2",
			expectedIP:         "10.0.0.1",
			expectedPort:       "12345",
			expectedAuthMethod: int(usersession.SSHAuthMethodPassword),
			expectedPublicKey:  "",
			expectedSSHDPid:    "5678",
			shouldAddToLRU:     true,
		},
		{
			name:           "Invalid log line - not sshd",
			logLine:        "Nov  5 09:59:44 lima-centos7-9-x64 sudo: Accepted publickey for lima from 192.168.5.2 port 38835 ssh2: ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU",
			shouldAddToLRU: false,
		},
		{
			name:           "Invalid log line - too short",
			logLine:        "Nov  5 09:59:44",
			shouldAddToLRU: false,
		},
		{
			name:           "Invalid log line - not an Accepted event",
			logLine:        "Nov  5 09:59:44 lima-centos7-9-x64 sshd[5678]: Failed password for invalid user admin from 192.168.5.2 port 38835 ssh2",
			shouldAddToLRU: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sshSessionParsed, err := lru.New[SSHSessionKey, SSHSessionValue](100)
			assert.NoError(t, err)

			parseSSHLogLine(tt.logLine, sshSessionParsed)

			if tt.shouldAddToLRU {
				key := SSHSessionKey{
					SSHDPid: tt.expectedSSHDPid,
					IP:      tt.expectedIP,
					Port:    tt.expectedPort,
				}

				value, found := sshSessionParsed.Get(key)
				assert.True(t, found, "SSH Session must be in LRU")

				if found {
					assert.Equal(t, tt.expectedAuthMethod, value.AuthenticationMethod, "Authentication method must match")
					assert.Equal(t, tt.expectedPublicKey, value.PublicKey, "Public key must match")
				}
			} else {
				assert.Equal(t, 0, sshSessionParsed.Len(), "LRU must be empty")
			}
		})
	}
}
