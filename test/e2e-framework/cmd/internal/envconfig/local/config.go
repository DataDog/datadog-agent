// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package local describes the local environment's input contract. The
// environment is the developer's host: there is nothing to provision beyond
// the common fakeintake toggle, so the type is deliberately empty. The
// fakeintake must be enabled (real-backend selection is the receiver plan's
// feature, not this environment's).
package local

import "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"

type Config struct{}

var Schema = configschema.Must[Config]()
