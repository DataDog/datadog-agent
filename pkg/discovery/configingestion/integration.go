// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configingestion

import (
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

var generatedNameToIntegration = map[string]string{
	"redis-server": "redis",
	"mysqld":       "mysql",
	"postgres":     "postgresql",
	"mongod":       "mongodb",
	"nginx":        "nginx",
}

func integrationFromService(svc *workloadmeta.Service) string {
	if svc == nil {
		return "unknown"
	}
	if mapped, ok := generatedNameToIntegration[strings.ToLower(svc.GeneratedName)]; ok {
		return mapped
	}
	return strings.ToLower(svc.GeneratedName)
}
