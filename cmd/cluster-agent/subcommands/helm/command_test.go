// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRollbackCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"helm", "rollback",
			"--release", "myrel",
			"--namespace", "prod",
			"--job-namespace", "ops",
			"--service-account", "helm-sa",
			"--revision", "3"},
		runRollback,
		func(params *rollbackCliParams) {
			assert.Equal(t, "myrel", params.release)
			assert.Equal(t, "prod", params.releaseNamespace)
			assert.Equal(t, "ops", params.jobNamespace)
			assert.Equal(t, "helm-sa", params.serviceAccount)
			assert.Equal(t, 3, params.revision)
		})
}

func TestRollbackCommand_RequiredFlags(t *testing.T) {
	cmd := rollbackCmd(&command.GlobalParams{})
	cmd.SetArgs([]string{}) // no flags provided
	err := cmd.Execute()
	assert.Error(t, err, "expected an error when required flags are missing")
}
