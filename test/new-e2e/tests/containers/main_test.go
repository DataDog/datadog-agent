// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
)

var keepStacks = flag.Bool("keep-stacks", false, "Do not destroy the Pulumi stacks at the end of the tests")

func TestMain(m *testing.M) {
	code := m.Run()
	initOnly, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.InitOnly, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	if runner.GetProfile().AllowDevMode() && *keepStacks || initOnly {
		fmt.Fprintln(os.Stderr, "Keeping stacks")
	} else {
		fmt.Fprintln(os.Stderr, "Cleaning up stacks")
		errs := infra.GetStackManager().Cleanup(context.Background())
		for _, err := range errs {
			fmt.Fprint(os.Stderr, err.Error())
		}
	}
	os.Exit(code)
}
