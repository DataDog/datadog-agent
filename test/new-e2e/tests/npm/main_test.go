// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 {
		fmt.Fprintf(os.Stderr, "Cleaning up stacks")
		errs := infra.GetStackManager().Cleanup(context.Background())
		for _, err := range errs {
			fmt.Fprint(os.Stderr, err.Error())
		}
	}
	os.Exit(code)
}
