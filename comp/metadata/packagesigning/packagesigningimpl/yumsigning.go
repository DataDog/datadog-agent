// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagesigningimpl

import (
	"bufio"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	yumConf  = "/etc/yum.conf"
	yumRepo  = "/etc/yum.repos.d/"
	zyppConf = "/etc/zypp/zypp.conf"
	zyppRepo = "/etc/zypp/repos.d/"
)

// getMainGPGCheck returns gpgcheck setting for [main] table
func getMainGPGCheck(pkgManager string) bool {
	repoConfig, _ := getRepoPathFromPkgManager(pkgManager)
	gpgCheck, _ := parseRepoFile(repoConfig, false)
	return gpgCheck
}

// getYUMSignatureKeys returns the list of keys used to sign RPM packages
func getYUMSignatureKeys(pkgManager string) []SigningKey {
	allKeys := make(map[SigningKey]struct{})
	getKeysFromRepoFiles(allKeys, pkgManager)
	getKeysFromRPMDB(allKeys)
	var keyList []SigningKey
	for keys := range allKeys {
		keyList = append(keyList, keys)
	}
	return keyList
}

func getKeysFromRepoFiles(allKeys map[SigningKey]struct{}, pkgManager string) {
	var gpgCheck bool
	var gpgFiles []string
	repoConfig, repoFilesDir := getRepoPathFromPkgManager(pkgManager)

	// First parsing of the main config file
	if _, err := os.Stat(repoConfig); os.IsExist(err) {
		gpgCheck, gpgFiles = parseRepoFile(repoConfig, false)
		for _, gpgFile := range gpgFiles {
			decryptGPGFile(allKeys, gpgFile, "repo")
		}
	}
	// Then parsing of the repo files
	if _, err := os.Stat(repoFilesDir); os.IsExist(err) {
		if files, err := os.ReadDir(repoFilesDir); err == nil {
			for _, file := range files {
				_, gpgFiles = parseRepoFile(file.Name(), gpgCheck)
				for _, gpgFile := range gpgFiles {
					decryptGPGFile(allKeys, gpgFile, "repo")
				}
			}
		}
	}
}

// getRepoPathFromPkgManager returns the path to the configuration file and the path to the repository files for RH or SUSE based OS
func getRepoPathFromPkgManager(pkgManager string) (string, string) {
	if pkgManager == "zypper" {
		return zyppConf, zyppRepo
	}
	return yumConf, yumRepo
}

func getKeysFromRPMDB(allKeys map[SigningKey]struct{}) {
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
		decryptGPGReader(allKeys, strings.NewReader(string(rpmKey)), "rpm")
	}
}

// parseRepoFile extracts information from yum repo files
// Save the global gpgcheck value when encountering a [main] table (should only occur on `/etc/yum.conf`)
// Match several entries in gpgkey field, either file references (file://) or http(s)://. From observations,
// these reference can be separated either by space or by new line. We assume it possible to mix file and http references
func parseRepoFile(inputFile string, gpgConf bool) (bool, []string) {

	file, err := os.Open(inputFile)
	if err != nil {
		return true, nil
	}
	defer file.Close()

	signedBy := regexp.MustCompile(`file://([A-Za-z0-9_\-\.\/]+)`)
	table := regexp.MustCompile(`\[[A-Za-z0-9_\-\.\/ ]+\]`)
	urlmatch := regexp.MustCompile(`http(s)?://[A-Za-z0-9_\-\.\/]+`)
	gpgCheck := gpgConf
	localGpgCheck := false
	inMain := false
	var gpgFiles []string
	var localGpgFiles []string
	defer file.Close()
	if err != nil {
		return true, nil
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		currentTable := table.FindString(line)
		if currentTable == "[main]" {
			inMain = true
		} else if currentTable != "" {
			// entering new table, save values
			if localGpgCheck {
				gpgFiles = append(gpgFiles, localGpgFiles...)
			}
			// reset
			inMain = false
			localGpgCheck = gpgCheck
			localGpgFiles = nil
		}
		if strings.HasPrefix(line, "gpgcheck") {
			if inMain {
				gpgCheck = strings.Contains(line, "1")
			} else {
				localGpgCheck = strings.Contains(line, "1")
			}
		}
		if strings.HasPrefix(line, "gpgkey") {
			// check format `gpgkey=file///etc/file1 file:///etc/file2`
			signedFile := signedBy.FindAllStringSubmatch(line, -1)
			for _, match := range signedFile {
				if len(match) > 1 {
					localGpgFiles = append(localGpgFiles, match[1])
				}
			}
			urls := urlmatch.FindAllString(strings.TrimPrefix(line, "gpgkey="), -1)
			localGpgFiles = append(localGpgFiles, urls...)
			// Scan other lines in case of multiple gpgkey
			for scanner.Scan() {
				cont := scanner.Text()
				// Assuming continuation lines are indented
				if !strings.HasPrefix(cont, " ") && !strings.HasPrefix(cont, "\t") {
					tbl := table.FindString(cont)
					if tbl != "" {
						// entering new table, save values
						if localGpgCheck {
							gpgFiles = append(gpgFiles, localGpgFiles...)
						}
						// reset
						inMain = false
						localGpgCheck = gpgCheck
						localGpgFiles = nil
					}
					break
				}
				signedFile := signedBy.FindAllStringSubmatch(cont, -1)
				for _, match := range signedFile {
					if len(match) > 1 {
						localGpgFiles = append(localGpgFiles, match[1])
					}
				}
				urls := urlmatch.FindAllString(strings.TrimSpace(cont), -1)
				localGpgFiles = append(localGpgFiles, urls...)

			}
		}
	}
	// save last values
	if localGpgCheck {
		gpgFiles = append(gpgFiles, localGpgFiles...)
	}
	return gpgCheck, gpgFiles
}
