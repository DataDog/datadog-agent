// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package snmptraps implements the a server that listens for SNMP trap data
// and sends it to the backend.
package snmptraps

import (
	"github.com/DataDog/datadog-agent/comp/snmptraps/config/configimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter/formatterimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener/listenerimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver/oidresolverimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/server/serverimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status/statusimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: network-device-monitoring

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	configimpl.Module,
	formatterimpl.Module,
	forwarderimpl.Module,
	listenerimpl.Module,
	oidresolverimpl.Module,
	statusimpl.Module,
	serverimpl.Module,
)
