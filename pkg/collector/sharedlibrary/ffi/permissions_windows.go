// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package ffi

import (
	secretsimpl "github.com/DataDog/datadog-agent/comp/core/secrets/impl"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

func checkOwnerAndPermissions(path string) error {
	p, err := filesystem.NewPermission()
	if err != nil {
		return err
	}

	if err := p.CheckOwner(path); err != nil {
		return err
	}

	return secretsimpl.CheckRights(path, false)
}
