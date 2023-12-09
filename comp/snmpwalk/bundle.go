// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package snmpwalk implements the "snmpwalk" bundle, which collect partial snmpwalk
// for runtime SNMP integration instances
package snmpwalk

import (
	configimpl "github.com/DataDog/datadog-agent/comp/snmpwalk/config/impl"
	serverimpl "github.com/DataDog/datadog-agent/comp/snmpwalk/server/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: network-device-monitoring

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	serverimpl.Module,
	configimpl.Module,
)
