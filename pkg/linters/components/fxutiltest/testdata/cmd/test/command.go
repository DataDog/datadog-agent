// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package test is a test package for fxutil linter
package test

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func myFunction() error {
	return nil
}

func dumpContexts() error {
	return nil
}

func Commands() []*cobra.Command {
	return []*cobra.Command{
		{
			Use: "top",
			RunE: func(_ *cobra.Command, _ []string) error {
				return fxutil.OneShot(myFunction)
			},
		},
		{ // want `Cobra command 'dump-contexts' uses fxutil.OneShot but has no corresponding test`
			Use: "dump-contexts",
			RunE: func(_ *cobra.Command, _ []string) error {
				return fxutil.OneShot(dumpContexts)
			},
		},
	}
}
