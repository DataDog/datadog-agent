// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build !linux && !windows

package consumer

import (
	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Copy the event
func (p *ProcessConsumer) Copy(_ *smodel.Event) any {
	return nil
}