// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogstatsd contains the dogstatsd subcommands.
package dogstatsd

import (
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
)

// DSD is the global dogstatsd instance
// TODO: (components) This is currently only used by JMXFetch.
// Once core check runners are refactored to not use init hooks,
// we can remove this global.
var DSD server.Component
