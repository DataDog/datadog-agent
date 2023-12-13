// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package winevtapi

//revive:disable:var-naming These names are intended to match the Windows API names

// EVT_LOGIN_CLASS
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_login_class
const (
	EvtRpcLogin = 1
)

// EVT_RPC_LOGIN is a C struct used when calling EvtOpenSession
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/ns-winevt-evt_rpc_login
type EVT_RPC_LOGIN struct {
	Server   *uint16
	User     *uint16
	Domain   *uint16
	Password *uint16
	Flags    uint
}

//revive:enable:var-naming
