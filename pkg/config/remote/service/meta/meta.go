// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package meta

import _ "embed"

// RootConfig is the root of the config repo
//go:embed config.json
var RootConfig []byte

// RootDirector is the root of the director repo
//go:embed director.json
var RootDirector []byte
