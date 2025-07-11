// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package generic contains utility functions for creating filterable objects.
package generic

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	typedef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def/proto"
)

// CreateImage creates a Filterable Image object.
func CreateImage(name string) *workloadfilter.Image {
	return &workloadfilter.Image{
		FilterImage: &typedef.FilterImage{
			Name: name,
		},
	}
}
