// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func ReadSecretFile(filePath string) (pulumi.StringOutput, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	s := pulumi.ToSecret(pulumi.String(string(b))).(pulumi.StringOutput)

	return s, nil
}
