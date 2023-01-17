// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"github.com/spf13/cobra"
)

type GlobalParams struct {
	ConfigFilePaths []string
}

type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

const LoggerName = "SECURITY"
