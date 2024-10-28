// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"fmt"
	"strings"
)

type catalog struct {
	composeFilesByProjects map[string]string
}

var defaultCatalog = catalog{
	composeFilesByProjects: make(map[string]string),
}

func (c *catalog) addCompose(projectName, filename string) {
	c.composeFilesByProjects[projectName] = filename
}

func registerComposeFile(filename string) {
	defaultCatalog.addCompose(strings.TrimSuffix(filename, ".compose"), fmt.Sprintf("testdata/%s", filename))
}
