// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package create

import "github.com/DataDog/datadog-agent/pkg/config/model"

// IsViperBacked reports whether the config impl reads through viper (directly
// or via a teeconfig wrapper).
func IsViperBacked(b model.Reader) bool {
	type libTyper interface{ GetLibType() string }
	lt, ok := b.(libTyper)
	return ok && lt.GetLibType() == "viper"
}
