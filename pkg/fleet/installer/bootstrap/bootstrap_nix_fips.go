// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build goexperiment.systemcrypto && !windows

package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
)

// fipsInstallerEnv returns the OpenSSL FIPS provider environment that the
// bootstrap installer process needs to start.
//
// The installer extracted from an agent package's installer layer is a bare
// binary built with the requirefips tag: it aborts during init unless a
// self-tested OpenSSL FIPS provider is available, and it has no package tree of
// its own to find one in. Point it at the FIPS tree of the currently-running
// installer instead, whose openssl.cnf + fipsmodule.cnf were already generated
// for this machine (the running process is itself a working FIPS binary). This
// mirrors the OPENSSL_CONF + OPENSSL_MODULES convention used elsewhere to enable
// FIPS in a process.
//
// Best-effort: if the expected provider files are not where we derive them, we
// return no extra environment rather than forcing a broken config, leaving the
// process defaults in place.
func fipsInstallerEnv() ([]string, error) {
	exePath, err := exec.GetExecutable()
	if err != nil {
		return nil, fmt.Errorf("could not resolve running installer path: %w", err)
	}
	// <install>/embedded/bin/installer -> <install>/embedded
	embedded := filepath.Dir(filepath.Dir(exePath))
	opensslConf := filepath.Join(embedded, "ssl", "openssl.cnf")
	opensslModules := filepath.Join(embedded, "lib", "ossl-modules")
	for _, p := range []string{opensslConf, opensslModules} {
		if _, statErr := os.Stat(p); statErr != nil {
			return nil, nil
		}
	}
	return []string{
		"OPENSSL_CONF=" + opensslConf,
		"OPENSSL_MODULES=" + opensslModules,
	}, nil
}
