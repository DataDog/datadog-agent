// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !docker

package tailerfactory

import (
	"errors"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type factory struct{}

func (tf *factory) makeSocketTailer(source *sources.LogSource) (Tailer, error) {
	return nil, errors.New("socket tailing is unavailable")
}
