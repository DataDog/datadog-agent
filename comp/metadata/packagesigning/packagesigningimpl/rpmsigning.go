// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagesigningimpl

import (
	"bufio"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	yumConf  = "/etc/yum.conf"
	yumRepo  = "/etc/yum.repos.d/"
	dnfConf  = "/etc/dnf/dnf.conf"
	zyppConf = "/etc/zypp/zypp.conf"
	zyppRepo = "/etc/zypp/repos.d/"
)

// getMainGPGCheck returns gpgcheck and repo_gpgcheck setting for [main] table
func getMainGPGCheck(pkgManager string) (bool, bool) {
	repoConfig, _ := getRepoPathFromPkgManager(pkgManager)
	if repoConfig == "" {
		// if we end up in a non supported distribution
		return false, false
	}
	mainConf, _ := parseRepoFile(repoConfig, mainData{})
	return mainConf.gpgcheck, mainConf.repoGpgcheck
}

// getYUMSignatureKeys returns the list of keys used to sign RPM packages
func getYUMSignatureKeys(pkgManager string, client *http.Client) []SigningKey {
	allKeys := make(map[string]SigningKey)
	updateWithRepoFiles(allKeys, pkgManager, client)
	updateWithRPMDB(allKeys)
	var keyList []SigningKey
	for _, key := range allKeys {
		keyList = append(keyList, key)
	}
	return keyList
}

func updateWithRepoFiles(allKeys map[string]SigningKey, pkgManager string, client *http.Client) {
	var mainConf mainData
	var reposPerKey map[string][]repositories
	repoConfig, repoFilesDir := getRepoPathFromPkgManager(pkgManager)
	if repoConfig == "" {
		// if we end up in a non supported distribution
		return
	}

	// First parsing of the main config file
	if _, err := os.Stat(repoConfig); !os.IsNotExist(err) {
		mainConf, reposPerKey = parseRepoFile(repoConfig, mainData{})
		for name, repos := range reposPerKey {
			decryptGPGFile(allKeys, repoFile{name, repos}, "signed-by", client)
		}
	}
	// Then parsing of the repo files
	if _, err := os.Stat(repoFilesDir); !os.IsNotExist(err) {
		if files, err := os.ReadDir(repoFilesDir); err == nil {
			for _, file := range files {
				_, reposPerKey := parseRepoFile(file.Name(), mainConf)
				for name, repos := range reposPerKey {
					decryptGPGFile(allKeys, repoFile{name, repos}, "signed-by", client)
				}
			}
		}
	}
}

// getRepoPathFromPkgManager returns the path to the configuration file and the path to the repository files for RH or SUSE based OS
func getRepoPathFromPkgManager(pkgManager string) (string, string) {
	if pkgManager == "yum" {
		return yumConf, yumRepo
	} else if pkgManager == "dnf" {
		return dnfConf, yumRepo
	} else if pkgManager == "zypper" {
		return zyppConf, zyppRepo
	}
	return "", ""
}

func updateWithRPMDB(allKeys map[string]SigningKey) {
	// It seems not possible to get the expiration date from rpmdb, so we extract the list of keys and call gpg
	cmd := exec.Command("rpm", "-qa", "gpg-pubkey*")
	output, err := cmd.Output()
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		rpmCmd := exec.Command("rpm", "-qi", "%s", "--qf", "'%%{PUBKEYS}\n'")
		rpmKey, err := rpmCmd.Output()
		if err != nil {
			return
		}
		decryptGPGReader(allKeys, strings.NewReader(string(rpmKey)), "rpm", nil)
	}
}

type mainData struct {
	gpgcheck         bool
	localpkgGpgcheck bool
	repoGpgcheck     bool
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

// parseRepoFile extracts information from yum repo files
// Save the global gpgcheck value when encountering a [main] table (should only occur on `/etc/yum.conf`)
// Match several entries in gpgkey field, either file references (file://) or http(s)://. From observations,
// these reference can be separated either by space or by new line. We assume it possible to mix file and http references
func parseRepoFile(inputFile string, mainConf mainData) (mainData, map[string][]repositories) {
	main := mainConf
	file, err := os.Open(inputFile)
	if err != nil {
		return main, nil
	}
	defer file.Close()

	reposPerKey := make(map[string][]repositories)
	table := regexp.MustCompile(`\[[A-Za-z0-9_\-\.\/ ]+\]`)
	field := regexp.MustCompile(`^([a-z_]+)\s?=\s?(.*)`)
	nextLine := multiLine{inside: false, name: ""}
	repo := repoData{enabled: true}
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
					main.gpgcheck = matches[2][0] == '1'
				case "localpkg_gpgcheck":
					main.localpkgGpgcheck = matches[2][0] == '1'
				case "repo_gpgcheck":
					main.repoGpgcheck = matches[2][0] == '1'
				}
			} else { // in repo
				if fieldName == "enabled" {
					repo.enabled = matches[2][0] == '1'
				} else if fieldName == "gpgcheck" {
					repo.gpgcheck = matches[2][0] == '1'
				} else if fieldName == "repo_gpgcheck" {
					repo.repoGpgcheck = matches[2][0] == '1'
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
		if repo.enabled && (main.gpgcheck || repo.gpgcheck) {
			for _, key := range repo.gpgkey {
				var r []repositories
				for _, baseurl := range repo.baseurl {
					r = append(r, repositories{RepoName: baseurl})
				}
				if repo.mirrorlist != "" {
					r = append(r, repositories{RepoName: repo.mirrorlist})
				}
				if v, ok := reposPerKey[key]; !ok {
					reposPerKey[key] = r
				} else {
					reposPerKey[key] = append(v, r...)
				}
			}
		}
	}
	return main, reposPerKey
}
