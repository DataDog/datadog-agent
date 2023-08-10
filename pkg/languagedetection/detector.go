// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

type Detector interface {
	DetectLanguage(pid int) (languagemodels.Language, error)
}
