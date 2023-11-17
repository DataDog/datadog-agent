// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cache

import (
	"go.uber.org/fx"
)

// Module for testing components that need KeyedStringInterners.
var Module = fx.Module("cache", fx.Provide(NewKeyedStringInterner))
