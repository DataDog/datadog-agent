// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build tools

package tools

// These imports are used to track go:generate dependencies.
// This is the currently recommended approach: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

import (
	_ "github.com/go-delve/delve/pkg/goversion"
	_ "github.com/mailru/easyjson/easyjson"
	_ "github.com/tinylib/msgp"
	_ "golang.org/x/tools/cmd/stringer"

	_ "github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	_ "github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils"
	_ "github.com/DataDog/datadog-agent/pkg/network/go/lutgen"
)
