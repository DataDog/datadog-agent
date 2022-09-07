// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets
// +build secrets

package secrethelper

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/cmd/secrets"
)

// Command returns the main cobra config command.
func Command(globalArgs *app.GlobalArgs) *cobra.Command {
	// TODO: move to cmd/common/secrethelper?
	return secrets.SecretHelperCmd
}
