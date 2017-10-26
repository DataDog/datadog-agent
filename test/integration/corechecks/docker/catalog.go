// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package docker

type catalog struct {
	composeFiles map[string]string
}

var defaultCatalog = catalog{
	composeFiles: make(map[string]string),
}

func registerComposeFile(name string, filename string) {
	defaultCatalog.composeFiles[name] = filename
}
