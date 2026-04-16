// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apt

import (
	"bufio"
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

func parseAPTListLine(line string) *repository {
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

func parseAPTListFile(configPath string) (repositoryList, error) {
	rdr, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %s", configPath, err)
	}
	defer rdr.Close()

	res := repositoryList{}
	scanner := bufio.NewScanner(rdr)
	for scanner.Scan() {
		line := scanner.Text()
		repo := parseAPTListLine(line)
		if repo != nil {
			repo.configFile = configPath
			res = append(res, repo)
		}
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("read %s: %s", configPath, err)
	}
	return res, nil
}

func parseAPTSourcesFile(configPath string) (repositoryList, error) {
	rdr, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %s", configPath, err)
	}
	defer rdr.Close()

	res := repositoryList{}
	repo := repository{Enabled: true, configFile: configPath}
	var repoTypes []string
	var uris []string
	var suites []string
	var options []string

	var expandValues = func() {
		if len(repoTypes) > 0 && len(uris) > 0 && len(suites) > 0 {
			repo.Options = strings.Join(options, " ")
			for _, repoType := range repoTypes {
				for _, uri := range uris {
					for _, suite := range suites {
						repoCopy := repo
						repoCopy.SourceRepo = repoType == "deb-src"
						repoCopy.URI = uri
						repoCopy.Distribution = suite
						res = append(res, &repoCopy)
					}
				}
			}
		}
	}

	scanner := bufio.NewScanner(rdr)
	for scanner.Scan() {
		line := scanner.Text()
		// end of repo
		if strings.TrimSpace(line) == "" {
			expandValues()
			repo = repository{Enabled: true, configFile: configPath}
			clear(repoTypes)
			clear(uris)
			clear(suites)
			clear(options)
			continue
		}
		if line[0] == '#' {
			continue
		}
		// TODO this doesn't support folded or multiline field values
		// https://manpages.debian.org/buster/dpkg-dev/deb822.5.en.html

		stanza, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)

		switch strings.ToLower(stanza) {
		case "types":
			repoTypes = strings.Split(value, " ")
		case "uris":
			uris = strings.Split(value, " ")
		case "suites":
			suites = strings.Split(value, " ")
		case "components":
			repo.Components = value
		case "enabled":
			repo.Enabled = value != "no"
		default:
			options = append(options, stanza+"="+value)
		}
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("read %s: %s", configPath, err)
	}

	// if there isn't an empty line at the end, make sure to still use pending values
	expandValues()
	return res, nil
}

// parseAPTConfigFolder scans an APT config folder (usually /etc/apt) to
// get information about all configured repositories, it scans also
// "source.list.d" subfolder to find all the "*.list" files.
func parseAPTConfigFolder(folderPath string) (repositoryList, error) {
	var sourcesFiles []string
	listFiles := []string{filepath.Join(folderPath, "sources.list")}
	sourcesFolder := filepath.Join(folderPath, "sources.list.d")
	dfiles, err := os.ReadDir(sourcesFolder)
	if err != nil {
		return nil, fmt.Errorf("read %s folder: %s", sourcesFolder, err)
	}
	for _, l := range dfiles {
		if strings.HasSuffix(l.Name(), ".list") {
			listFiles = append(listFiles, filepath.Join(sourcesFolder, l.Name()))
		}
		if strings.HasSuffix(l.Name(), ".sources") {
			sourcesFiles = append(sourcesFiles, filepath.Join(sourcesFolder, l.Name()))
		}
	}

	res := repositoryList{}
	for _, listFile := range listFiles {
		repos, err := parseAPTListFile(listFile)
		if err != nil {
			return nil, fmt.Errorf("list parse %s: %s", listFile, err)
		}
		res = append(res, repos...)
	}
	for _, sourcesFile := range sourcesFiles {
		repos, err := parseAPTSourcesFile(sourcesFile)
		if err != nil {
			return nil, fmt.Errorf("sources parse %s: %s", sourcesFile, err)
		}
		res = append(res, repos...)
	}
	return res, nil
}
