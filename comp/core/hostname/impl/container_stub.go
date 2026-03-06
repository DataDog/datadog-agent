// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build freebsd || netbsd || openbsd || solaris || dragonfly || aix

package hostnameimpl

import (
	"context"
	"fmt"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func fromContainer(_ context.Context, _ pkgconfigmodel.Reader, _ string) (string, error) {
	return "", fmt.Errorf("container support is not compiled in")
}
