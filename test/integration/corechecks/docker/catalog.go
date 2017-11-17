// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package docker

type catalog struct {
	composeFiles []string
}

var defaultCatalog catalog

func (c *catalog) addCompose(filename string) {
	c.composeFiles = append(c.composeFiles, filename)
}

func registerComposeFile(filename string) {
	defaultCatalog.addCompose(filename)
}
