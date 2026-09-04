// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package driver

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/drivers/ec2host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/drivers/kind"
)

// registry is THE edit site for adding an environment: implement
// driver.Driver in a package under internal/drivers, and add one line here.
// Explicit by decision (T1): greppable, no init magic.
var registry = []Driver{
	&kind.Driver{},
	&ec2host.Driver{},
}
