// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package avd maps local RDP sessions into the shared VDI inventory.
// The check's explicit provider configuration identifies the host as AVD;
// WTS itself does not distinguish AVD from other RDP session hosts.
package avd

import (
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/vdi/model"
)

// FromWindowsInventory preserves the source status and maps desktop sessions.
// Listener, service, and console sessions are not AVD user sessions. A
// disconnected desktop remains a session even though it has no live client.
func FromWindowsInventory(windows model.WindowsInventory) model.ProviderInventory {
	result := model.ProviderInventory{SourceStatus: windows.SourceStatus}
	if windows.Status != model.StatusOK && windows.Status != model.StatusPartial {
		return result
	}
	for _, session := range windows.Sessions {
		if session.WindowsSessionID == 0 {
			continue
		}
		switch session.State {
		case "active", "connected", "connect_query", "shadow", "disconnected", "idle":
		default:
			continue
		}
		if strings.EqualFold(session.Name, "console") || strings.EqualFold(session.Name, "services") {
			continue
		}
		// Client protocol describes the current connection. On a disconnected
		// AVD desktop, Windows can report protocol 0 and no remote-session flag.
		// Keep the logged-on desktop independently of its vanished connection.
		if session.State == "disconnected" {
			if session.User == "" && session.LogonAt == nil && session.Protocol != model.ProtocolRDP {
				continue
			}
		} else if session.Protocol == "" {
			result.Status = model.StatusPartial
			result.Error = "RDP protocol could not be determined for one or more Windows sessions"
			continue
		} else if session.Protocol != model.ProtocolRDP {
			continue
		}
		user := session.User
		if user != "" && session.Domain != "" && !strings.ContainsAny(user, "\\@") {
			user = session.Domain + "\\" + user
		}
		result.Sessions = append(result.Sessions, model.Session{
			ID:          strconv.FormatUint(uint64(session.WindowsSessionID), 10),
			Protocol:    model.ProtocolRDP,
			User:        user,
			State:       session.State,
			LogonAt:     session.LogonAt,
			LastInputAt: session.LastInputAt,
		})
	}
	return result
}
