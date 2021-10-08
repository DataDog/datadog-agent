// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package metadata implements specific Metadata Collectors for the Agent. Such
collectors might have dependencies (like Python) that we don't want in the
general purpose `github.com/DataDog/datadog-agent/pkg/metadata` package,
that can be imported by different softwares like Dogstatsd.
*/
package metadata
