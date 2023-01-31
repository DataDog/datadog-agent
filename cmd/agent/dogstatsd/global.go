// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
)

// DSD is the global dogstatsd instance
// TODO: (components) This should be removed when all downstream dependencies
// are migrated to components such that this can be injected instead of a shared
// global reference.
var DSD server.Component
