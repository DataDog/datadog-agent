// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package generatedsettings registers the Agent settings declared in the config schema.
package generatedsettings

// The rest of this package is generated from the schema. Importing its dependencies here keeps
// them in go.mod, and the package resolvable, before `dda inv schema.codegen` has run.
import (
	_ "github.com/DataDog/datadog-agent/pkg/config/env"
	_ "github.com/DataDog/datadog-agent/pkg/config/helper"
	_ "github.com/DataDog/datadog-agent/pkg/config/model"
)
