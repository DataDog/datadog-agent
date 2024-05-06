// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import "github.com/DataDog/datadog-agent/pkg/security/events"

// Opts define module options
type Opts struct {
	EventSender events.EventSender
	MsgSender   MsgSender
}
