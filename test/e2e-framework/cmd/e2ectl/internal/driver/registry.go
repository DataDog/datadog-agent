// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package driver

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/drivers/ec2host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/drivers/kind"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/drivers/local"
	ec2config "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/ec2host"
	kindconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/kind"
	localconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/local"
)

// Register a shared typed schema, an explicit default installer and lifecycle
// behavior. Description is required by Define. No handwritten YAML or mandatory
// driver Validate method is needed.
var registry = []Driver{
	Define(kindconfig.Schema, "helm", &kind.Driver{}),
	Define(ec2config.Schema, "script", &ec2host.Driver{}),
	Define(localconfig.Schema, "binary", &local.Driver{}),
}
