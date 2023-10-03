// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/DataDog/datadog-agent/pkg/conf/utils"
)

// SanitizeAPIKey strips newlines and other control characters from a given string.
var SanitizeAPIKey = utils.SanitizeAPIKey
