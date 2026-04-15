// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apt

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// repositoryList is an array of repository definitions
type repositoryList []*repository

// repository contains metadata about a repository installed in the system
type repository struct {
	Enabled      bool
	SourceRepo   bool
	Options      string
	URI          string
	Distribution string
	Components   string
	Comment      string

	configFile string
}

var aptConfigLineRegexp = regexp.MustCompile(`^(# )?(deb|deb-src)(?: \[(.*)])? ([^ \[]+) ([^ ]+)(?: ([^#\n]+))?(?: +# *(.*))?$`)

func parseAPTConfigLine(line string) *repository {
	match := aptConfigLineRegexp.FindAllStringSubmatch(line, -1)
	if len(match) == 0 || len(match[0]) < 6 {
		return nil
	}
	fields := match[0]
	return &repository{
		Enabled:      fields[1] != "# ",
		SourceRepo:   fields[2] == "deb-src",
		Options:      fields[3],
		URI:          fields[4],
		Distribution: fields[5],
		Components:   fields[6],
		Comment:      fields[7],
	}
}

func parseAPTConfigFile(configPath string) (repositoryList, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %s", configPath, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))

	res := repositoryList{}
	for scanner.Scan() {
		line := scanner.Text()
		repo := parseAPTConfigLine(line)
		if repo != nil {
			repo.configFile = configPath
			res = append(res, repo)
		}
	}
	return res, nil
}

// parseAPTConfigFolder scans an APT config folder (usually /etc/apt) to
// get information about all configured repositories, it scans also
// "source.list.d" subfolder to find all the "*.list" files.
func parseAPTConfigFolder(folderPath string) (repositoryList, error) {
	sources := []string{filepath.Join(folderPath, "sources.list")}

	sourcesFolder := filepath.Join(folderPath, "sources.list.d")
	list, err := os.ReadDir(sourcesFolder)
	if err != nil {
		return nil, fmt.Errorf("read %s folder: %s", sourcesFolder, err)
	}
	for _, l := range list {
		if strings.HasSuffix(l.Name(), ".list") {
			sources = append(sources, filepath.Join(sourcesFolder, l.Name()))
		}
	}

	res := repositoryList{}
	for _, source := range sources {
		repos, err := parseAPTConfigFile(source)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %s", source, err)
		}
		res = append(res, repos...)
	}
	return res, nil
}
