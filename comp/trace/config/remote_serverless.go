// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package config

import (
	"errors"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	corecompcfg "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func remote(corecompcfg.Component, string, authtoken.Component) (config.RemoteClient, error) {
	return nil, errors.New("remote configuration is not supported in serverless")
}
