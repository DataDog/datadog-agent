// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package oracle provides utilities to detect oracle cloud provider.
package oracle

import (
	"context"

	diagnoseComp "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

func init() {
	diagnoseComp.RegisterMetadataAvail("OracleCloud Metadata availability", diagnose)
}

// diagnose the oraclecloud metadata API availability
func diagnose() error {
	_, err := GetHostAliases(context.TODO())
	return err
}
