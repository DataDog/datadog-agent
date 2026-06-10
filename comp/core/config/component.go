// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import configdef "github.com/DataDog/datadog-agent/comp/core/config/def"

// Component is a type alias kept for backward compatibility with external packages
// that import this root package (e.g. opentelemetry-collector-contrib/pkg/datadog).
// TODO: remove this file once all external callers have migrated to comp/core/config/def.
type Component = configdef.Component
