// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import "go.uber.org/fx"

// fxAppTestOverride allows TestRunCommand and TestOneShotSubcommand to
// override the Run and OneShot functions.  It is always nil in production.
var fxAppTestOverride func(interface{}, []fx.Option) error
