// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build localdog_ui

package server

import (
	"embed"
	"io/fs"
)

// The web app build is copied here by `dda inv localdog.build --with-ui`.
//
//go:embed all:ui
var uiFiles embed.FS

func embeddedUI() fs.FS {
	sub, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
