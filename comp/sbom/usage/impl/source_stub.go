// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !trivy

package usageimpl

import (
	config "github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// NewTrivySource returns no index source: a build without Trivy scans nothing,
// so it has no file table to serve and stamps no usage.
func NewTrivySource(_ config.Component, _ option.Option[workloadmeta.Component]) (option.Option[IndexSource], error) {
	return option.None[IndexSource](), nil
}
