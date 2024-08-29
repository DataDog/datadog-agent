// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import pkgutilcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"

// Aliases for common errors and functions from the containers.image package
var (
	ErrEmptyImage    = pkgutilcontainersimage.ErrEmptyImage
	ErrImageIsSha256 = pkgutilcontainersimage.ErrImageIsSha256
)

var (
	SplitImageName = pkgutilcontainersimage.SplitImageName
)
