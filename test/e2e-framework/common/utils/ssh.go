// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"fmt"
	"os"
	"path"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	defaultPublicKeyFilePath = ".ssh/id_rsa.pub"
)

func GetSSHPublicKey(filePath string) (pulumi.StringOutput, error) {
	if filePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return pulumi.StringOutput{}, fmt.Errorf("unable to read SSH key, err: %v", err)
		}

		filePath = path.Join(homeDir, defaultPublicKeyFilePath)
	}

	return ReadSecretFile(filePath)
}
