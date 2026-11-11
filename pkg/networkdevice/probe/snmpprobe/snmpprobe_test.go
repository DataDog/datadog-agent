// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmpprobe

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/failure"
)

func v2cCredential() Credential {
	return Credential{ID: "cred-1", Version: "2c", Community: "public"}
}

func testOptions() Options {
	return Options{
		Port:        1,
		Timeout:     50 * time.Millisecond,
		Retries:     0,
		Credentials: []Credential{v2cCredential()},
	}
}

func TestValidateAcceptsAZeroTimeoutAndZeroRetries(t *testing.T) {
	opts := testOptions()
	opts.Timeout = 0
	opts.Retries = 0

	assert.NoError(t, opts.Validate())
}

func TestValidateAcceptsAVersion3Credential(t *testing.T) {
	opts := testOptions()
	opts.Credentials = []Credential{{
		ID:           "cred-3",
		Version:      "3",
		User:         "test-user",
		AuthProtocol: "sha",
		AuthKey:      "test-auth-key",
		PrivProtocol: "aes",
		PrivKey:      "test-priv-key",
	}}

	assert.NoError(t, opts.Validate())
}

func TestValidateRejectsWhatCannotBeProbed(t *testing.T) {
	tests := map[string]func(*Options){
		"port zero":        func(o *Options) { o.Port = 0 },
		"negative retries": func(o *Options) { o.Retries = -1 },
		"no credential":    func(o *Options) { o.Credentials = nil },
		"unknown version":  func(o *Options) { o.Credentials = []Credential{{ID: "c", Version: "4"}} },
		"unknown auth algo": func(o *Options) {
			o.Credentials = []Credential{{ID: "c", Version: "3", User: "test-user", AuthProtocol: "nope", AuthKey: "test-auth-key"}}
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			opts := testOptions()
			mutate(&opts)

			err := opts.Validate()
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidOptions)
		})
	}
}

func TestValidateNamesTheCredentialIDAndNotItsSecret(t *testing.T) {
	opts := testOptions()
	opts.Credentials = []Credential{{ID: "cred-1", Version: "9", Community: "s3cret-community"}}

	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cred-1")
	assert.NotContains(t, err.Error(), "s3cret-community")
}

func TestFingerprintIgnoresCredentialOrder(t *testing.T) {
	a := testOptions()
	a.Credentials = []Credential{{ID: "a", Version: "2c", Community: "public"}, {ID: "b", Version: "2c", Community: "other"}}
	b := testOptions()
	b.Credentials = []Credential{{ID: "b", Version: "2c", Community: "other"}, {ID: "a", Version: "2c", Community: "public"}}

	assert.Equal(t, a.Fingerprint(), b.Fingerprint())
}

func TestFingerprintChangesWithTheCredentialMaterial(t *testing.T) {
	a := testOptions()
	b := testOptions()
	b.Credentials = []Credential{{ID: "cred-1", Version: "2c", Community: "rotated"}}

	assert.NotEqual(t, a.Fingerprint(), b.Fingerprint())
}

func TestFingerprintHoldsNoCredentialMaterial(t *testing.T) {
	opts := testOptions()
	opts.Credentials = []Credential{{ID: "cred-1", Version: "2c", Community: "s3cret-community"}}

	fp := opts.Fingerprint()
	assert.True(t, strings.HasPrefix(fp, "snmp:"))
	assert.NotContains(t, fp, "s3cret-community")
}

func TestRunReportsATimeoutAsAReading(t *testing.T) {
	r := Run(context.Background(), "127.0.0.1", testOptions())

	require.NotNil(t, r)
	assert.False(t, r.Success)
	assert.Contains(t, []string{failure.Timeout, failure.ConnectionRefused}, r.FailureReason)
	assert.Contains(t, r.Error, "127.0.0.1")
	assert.Empty(t, r.CredID)
	assert.Empty(t, r.SysName)
}

func TestRunReturnsTheLastReadingWhenEveryCredentialFails(t *testing.T) {
	opts := testOptions()
	opts.Credentials = []Credential{
		{ID: "cred-1", Version: "2c", Community: "public"},
		{ID: "cred-2", Version: "2c", Community: "other"},
	}

	r := Run(context.Background(), "127.0.0.1", opts)

	require.NotNil(t, r)
	assert.False(t, r.Success)
	assert.Contains(t, []string{failure.Timeout, failure.ConnectionRefused}, r.FailureReason)
}

func TestRunWithNoCredentialIsAReadingAndNotAPanic(t *testing.T) {
	opts := testOptions()
	opts.Credentials = nil

	r := Run(context.Background(), "127.0.0.1", opts)

	require.NotNil(t, r)
	assert.False(t, r.Success)
	assert.Equal(t, failure.Unknown, r.FailureReason)
}

func TestRunStopsOnACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := Run(ctx, "127.0.0.1", testOptions())

	require.NotNil(t, r)
	assert.False(t, r.Success)
}

func sysNamePacket(status gosnmp.SNMPError, name string) *gosnmp.SnmpPacket {
	return &gosnmp.SnmpPacket{
		Error: status,
		Variables: []gosnmp.SnmpPDU{{
			Name:  "." + oidSysName,
			Type:  gosnmp.OctetString,
			Value: []byte(name),
		}},
	}
}

func TestReadingFromPacketReportsAnErrorStatusAsAFailure(t *testing.T) {
	r := readingFromPacket(sysNamePacket(gosnmp.AuthorizationError, ""), v2cCredential(), time.Millisecond, "10.0.0.1")

	assert.False(t, r.Success, "an error status is not a reachable device")
	assert.Equal(t, failure.AuthenticationFailed, r.FailureReason)
	assert.Empty(t, r.CredID, "a rejected credential is not the one that answered")
}

func TestReadingFromPacketClassifiesANonAuthErrorStatusAsUnknown(t *testing.T) {
	r := readingFromPacket(sysNamePacket(gosnmp.GenErr, ""), v2cCredential(), time.Millisecond, "10.0.0.1")

	assert.False(t, r.Success)
	assert.Equal(t, failure.Unknown, r.FailureReason)
}

func TestReadingFromPacketReadsTheDeviceNameOnSuccess(t *testing.T) {
	r := readingFromPacket(sysNamePacket(gosnmp.NoError, "router-1"), v2cCredential(), 3*time.Millisecond, "10.0.0.1")

	assert.True(t, r.Success)
	assert.Equal(t, failure.None, r.FailureReason)
	assert.Equal(t, "router-1", r.SysName)
	assert.Equal(t, "cred-1", r.CredID)
	assert.Equal(t, 3*time.Millisecond, r.RTT)
}

func TestReadingFromPacketHoldsNoCredentialMaterial(t *testing.T) {
	cred := Credential{ID: "cred-1", Version: "2c", Community: "s3cr3t-community"}

	r := readingFromPacket(sysNamePacket(gosnmp.AuthorizationError, ""), cred, time.Millisecond, "10.0.0.1")

	assert.NotContains(t, r.Error, "s3cr3t-community")
}
