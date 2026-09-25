// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package opener

import (
	"github.com/spf13/afero"

	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type sourceFileOpener struct {
	FileOpener
	noFollow FileOpener
	source   *sources.ReplaceableSource
}

// ForSource returns an opener that applies the current source's symlink policy.
// The policy is read for every open so source replacements take effect immediately.
func ForSource(fileOpener FileOpener, source *sources.ReplaceableSource) FileOpener {
	if source == nil {
		return fileOpener
	}
	return &sourceFileOpener{
		FileOpener: fileOpener,
		noFollow:   fileOpener.NoFollow(),
		source:     source,
	}
}

func (o *sourceFileOpener) OpenLogFile(path string) (afero.File, error) {
	if cfg := o.source.Config(); cfg != nil && cfg.NoFollow {
		return o.noFollow.OpenLogFile(path)
	}
	return o.FileOpener.OpenLogFile(path)
}
