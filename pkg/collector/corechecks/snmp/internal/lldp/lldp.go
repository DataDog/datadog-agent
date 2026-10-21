// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package lldp

import ndmlldp "github.com/DataDog/datadog-agent/pkg/networkdevice/lldp"

// ChassisIDSubtypeMap mapping to translate into human-readable value for LldpChassisIdSubtype
var ChassisIDSubtypeMap = ndmlldp.ChassisIDSubtypeMap

// PortIDSubTypeMap mapping to translate into human-readable value for LldpPortIdSubtype
var PortIDSubTypeMap = ndmlldp.PortIDSubTypeMap
