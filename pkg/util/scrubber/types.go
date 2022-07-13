// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package scrubber implements support for cleaning sensitive information out of strings
// and files.
//
// Subpackages of this package implement scrubbers for specific purposes.
package scrubber

import "github.com/DataDog/datadog-agent/pkg/util/scrubber/types"

// Scrubber re-exports the Scrubber type.
//
// This is required to avoid cyclic package dependencies, which Go does not
// support.
type Scrubber = types.Scrubber
