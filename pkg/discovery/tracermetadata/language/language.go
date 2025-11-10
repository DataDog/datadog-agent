// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package language bridges the tracer metadata and the language detection package.
package language

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

// GetLanguage returns the language of a process from the tracer metadata
func GetLanguage(meta tracermetadata.TracerMetadata) (languagemodels.Language, error) {
	var name languagemodels.LanguageName
	switch meta.TracerLanguage {
	case "cpp":
		name = languagemodels.CPP
	case "python":
		name = languagemodels.Python
	case "go":
		name = languagemodels.Go
	case "dotnet":
		name = languagemodels.Dotnet
	case "php":
		name = languagemodels.PHP
	case "nodejs":
		name = languagemodels.Node
	case "ruby":
		name = languagemodels.Ruby
	case "jvm":
		name = languagemodels.Java
	default:
		return languagemodels.Language{}, fmt.Errorf("unknown tracer language %s", meta.TracerLanguage)
	}

	return languagemodels.Language{
		Name: name,
	}, nil
}
