// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// Not const for testing purpose
var (
	YumConf = "/etc/yum.conf"
	YumRepo = "/etc/yum.repos.d/"
)

const (
	dnfConf  = "/etc/dnf/dnf.conf"
	zyppConf = "/etc/zypp/zypp.conf"
	zyppRepo = "/etc/zypp/repos.d/"
)

// getMainGPGCheck returns gpgcheck and repo_gpgcheck setting for [main] table
func getMainGPGCheck(pkgManager string) (bool, bool) {
	repoConfig, _ := GetRepoPathFromPkgManager(pkgManager)
	if repoConfig == "" {
		// if we end up in a non supported distribution
		return false, false
	}
	mainConf, _ := ParseRPMRepoFile(repoConfig, MainData{})
	return mainConf.Gpgcheck, mainConf.RepoGpgcheck
}

// GetRepoPathFromPkgManager returns the path to the configuration file and the path to the repository files for RH or SUSE based OS
func GetRepoPathFromPkgManager(pkgManager string) (string, string) {
	if pkgManager == "yum" {
		return YumConf, YumRepo
	} else if pkgManager == "dnf" {
		return dnfConf, YumRepo
	} else if pkgManager == "zypper" {
		return zyppConf, zyppRepo
	}
	return "", ""
}

// Repository is a struct to store the repo name
type Repository struct {
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	GPGCheck     bool   `json:"gpgcheck"`
	RepoGPGCheck bool   `json:"repo_gpgcheck"`
}

// MainData contains the global definitions of gpg checks
type MainData struct {
	Gpgcheck         bool
	LocalpkgGpgcheck bool
	RepoGpgcheck     bool
}
type repoData struct {
	baseurl      []string
	enabled      bool
	metalink     string
	mirrorlist   string
	gpgcheck     bool
	repoGpgcheck bool
	gpgkey       []string
}

type multiLine struct {
	inside bool
	name   string
}

// ParseRPMRepoFile extracts information from yum repo files
// Save the global gpgcheck value when encountering a [main] table (should only occur on `/etc/yum.conf`)
// Match several entries in gpgkey field, either file references (file://) or http(s)://. From observations,
// these reference can be separated either by space or by new line. We assume it possible to mix file and http references
func ParseRPMRepoFile(inputFile string, mainConf MainData) (MainData, map[string][]Repository) {
	main := mainConf
	file, err := os.Open(inputFile)
	if err != nil {
		return main, nil
	}
	defer file.Close()
	defaultValue := strings.Contains(inputFile, "zypp") // Settings are enabled by default on SUSE, disabled otherwise
	reposPerKey := make(map[string][]Repository)
	table := regexp.MustCompile(`\[[A-Za-z0-9_\-\.\/ ]+\]`)
	field := regexp.MustCompile(`^([a-z_]+)\s?=\s?(.*)$`)
	nextLine := multiLine{inside: false, name: ""}
	repo := repoData{enabled: true, gpgcheck: defaultValue, repoGpgcheck: defaultValue}
	var repos []repoData

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		currentTable := table.FindString(line)
		// Check entering a new table
		if currentTable == "[main]" {
			nextLine = multiLine{inside: true, name: "main"}
		} else if currentTable != "" {
			// entering new table, save values
			if repo.gpgkey != nil {
				repos = append(repos, repo)
				repo = repoData{enabled: true}
			}
			nextLine = multiLine{inside: false, name: ""}
		}
		// Analyse raws
		matches := field.FindStringSubmatch(line)
		if len(matches) > 1 { // detected a field definition
			nextLine.inside = false
			fieldName := matches[1]
			if nextLine.name == "main" {
				switch fieldName {
				case "gpgcheck":
					main.Gpgcheck = isEnabled(matches[2])
				case "localpkg_gpgcheck", "pkg_gpgcheck":
					main.LocalpkgGpgcheck = isEnabled(matches[2])
				case "repo_gpgcheck":
					main.RepoGpgcheck = isEnabled(matches[2])
				}
			} else { // in repo
				if fieldName == "enabled" {
					repo.enabled = isEnabled(matches[2])
				} else if fieldName == "gpgcheck" {
					repo.gpgcheck = isEnabled(matches[2])
				} else if fieldName == "repo_gpgcheck" {
					repo.repoGpgcheck = isEnabled(matches[2])
				} else if fieldName == "metalink" {
					repo.metalink = matches[2]
				} else if fieldName == "mirrorlist" {
					repo.mirrorlist = matches[2]
				} else if fieldName == "baseurl" { // there can be several values in the 2 last ones
					repo.baseurl = append(repo.baseurl, strings.Fields(matches[2])...)
					nextLine = multiLine{inside: true, name: fieldName}
				} else if fieldName == "gpgkey" {
					repo.gpgkey = append(repo.gpgkey, strings.Fields(matches[2])...)
					nextLine = multiLine{inside: true, name: fieldName}
				}
			}
		} else if nextLine.inside {
			if nextLine.name == "gpgkey" {
				repo.gpgkey = append(repo.gpgkey, strings.Fields(strings.TrimSpace(line))...)
			} else if nextLine.name == "baseurl" {
				repo.baseurl = append(repo.baseurl, strings.Fields(strings.TrimSpace(line))...)
			}
		}
	}
	// save last values
	repos = append(repos, repo)
	// Now denormalize the data
	for _, repo := range repos {
		for _, key := range repo.gpgkey {
			var r []Repository
			for _, baseurl := range repo.baseurl {
				r = append(r, Repository{Name: baseurl, GPGCheck: repo.gpgcheck, RepoGPGCheck: repo.repoGpgcheck, Enabled: repo.enabled})
			}
			if repo.mirrorlist != "" {
				r = append(r, Repository{Name: repo.mirrorlist, GPGCheck: repo.gpgcheck, RepoGPGCheck: repo.repoGpgcheck, Enabled: repo.enabled})
			}
			if v, ok := reposPerKey[key]; !ok {
				reposPerKey[key] = r
			} else {
				reposPerKey[key] = append(v, r...)
			}
		}
	}
	return main, reposPerKey
}

func isEnabled(value string) bool {
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
