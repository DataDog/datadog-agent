// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package connectivity defines the wire types shared by the connectivityCheck.
package connectivity

import (
	"errors"
)

var ErrInvalidRequest = errors.New("invalid request")

const (
	CheckPing = "ping"
	CheckSNMP = "snmp"
)

const (
	FailureNone                     = "none"
	FailureUnknown                  = "unknown"
	FailureUnreachable              = "unreachable"
	FailureTimeout                  = "timeout"
	FailureConnectionRefused        = "connection_refused"
	FailureHostUnreachable          = "host_unreachable"
	FailureNetworkUnreachable       = "network_unreachable"
	FailureAuthenticationFailed     = "authentication_failed"
	FailureDecryptionFailed         = "decryption_failed"
	FailureUnknownUser              = "unknown_user"
	FailureUnsupportedSecurityLevel = "unsupported_security_level"
	FailureUnknownEngineID          = "unknown_engine_id"
)

type PingOptions struct {
	Count        int   `json:"count"`
	IntervalMs   int   `json:"intervalMs"`
	TimeoutMs    int   `json:"timeoutMs"`
	UseRawSocket *bool `json:"useRawSocket,omitempty"`
}

type SNMPOptions struct {
	Port      int `json:"port"`
	TimeoutMs int `json:"timeoutMs"`
	Retries   int `json:"retries"`
}

type SNMPCredential struct {
	ID              string `json:"id"`
	Version         string `json:"version"`
	Community       string `json:"community,omitempty"`
	User            string `json:"user,omitempty"`
	AuthProtocol    string `json:"authProtocol,omitempty"`
	AuthKey         string `json:"authKey,omitempty"`
	PrivProtocol    string `json:"privProtocol,omitempty"`
	PrivKey         string `json:"privKey,omitempty"`
	ContextName     string `json:"contextName,omitempty"`
	ContextEngineID string `json:"contextEngineId,omitempty"`
}

type CheckResult struct {
	Success bool   `json:"success"`
	RttMs   *int64 `json:"rttMs,omitempty"`
	Error   string `json:"error,omitempty"`
}

type PingResult struct {
	CheckResult
	FailureReason string `json:"failureReason"`
}

type SNMPResult struct {
	CheckResult
	FailureReason string `json:"failureReason"`
	CredID        string `json:"credID,omitempty"`
	SysName       string `json:"sysName,omitempty"`
}

type DeviceResult struct {
	IPAddress  string      `json:"ipAddress"`
	PingResult *PingResult `json:"pingResult,omitempty"`
	SNMPResult *SNMPResult `json:"snmpResult,omitempty"`
}

type Request struct {
	Targets     []string         `json:"targets"`
	Checks      []string         `json:"checks"`
	PingOptions *PingOptions     `json:"pingOptions,omitempty"`
	SNMPOptions *SNMPOptions     `json:"snmpOptions,omitempty"`
	Workers     int              `json:"workers,omitempty"`
	Credentials []SNMPCredential `json:"credentials,omitempty"`
}

type Result struct {
	Devices []DeviceResult `json:"devices"`
}
