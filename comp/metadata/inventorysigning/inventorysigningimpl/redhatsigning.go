// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorysigningimpl

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// GetRedhatSignatureKeys returns the list of keys used to sign RPM packages
func GetRedhatSignatureKeys() []SigningKey {
	allKeys := getKeysFromRepoFiles()
	allKeys = append(allKeys, getKeysFromRPMDB()...)
	return allKeys
}

func getKeysFromRepoFiles() []SigningKey {
	var allKeys []SigningKey
	var gpgCheck bool
	var gpgFiles []gpgReference
	repoConfig := "/etc/yum.conf"
	if _, err := os.Stat(repoConfig); os.IsExist(err) {
		gpgCheck, gpgFiles = parseRepoFile(repoConfig, false)
		for _, file := range gpgFiles {
			var gpgCmd string
			if file.isHTTP {
				gpgCmd = fmt.Sprintf("gpg <(curl -S %s)", file.path)
			} else {
				gpgCmd = fmt.Sprintf("gpg %s", file.path)
			}
			allKeys = append(allKeys, decryptGPGFile(gpgCmd, "repo")...)
		}
	}
	repoFiles := "/etc/yum.repos.d/"
	if _, err := os.Stat(repoFiles); os.IsExist(err) {
		if files, err := os.ReadDir(repoFiles); err == nil {
			for _, file := range files {
				_, gpgFiles = parseRepoFile(file.Name(), gpgCheck)
				for _, f := range gpgFiles {
					var gpgCmd string
					if f.isHTTP {
						gpgCmd = fmt.Sprintf("gpg <(curl -S %s)", f.path)
					} else {
						gpgCmd = fmt.Sprintf("gpg %s", f.path)
					}
					allKeys = append(allKeys, decryptGPGFile(gpgCmd, "repo")...)
				}
			}
		}
	}
	return allKeys

}

func getKeysFromRPMDB() []SigningKey {
	// TODO handle SUSE
	var allKeys []SigningKey
	// It seems not possible to get the expiration date from rpmdb, so we extract the list of keys and call gpg
	cmd := exec.Command("rpm", "-qa", "gpg-pubkey*")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		gpgCmd := fmt.Sprintf("gpg <(rpm -qi %s --qf '%%{PUBKEYS}\n')", scanner.Text())
		allKeys = append(allKeys, decryptGPGFile(gpgCmd, "rpm")...)
	}
	return allKeys
}

type gpgReference struct {
	path   string
	isHTTP bool
}

// parseRepoFile extracts information from yum repo files
// Save the global gpgcheck value when encountering a [main] table (should only occur on `/etc/yum.conf`)
// Match several entries in gpgkey field, either file references (file://) or http(s)://. From observations,
// these reference can be separated either by space or by new line. We assume it possible to mix file and http references
func parseRepoFile(inputFile string, gpgConf bool) (bool, []gpgReference) {

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
	var gpgFiles []gpgReference
	var localGpgFiles []gpgReference
	defer file.Close()
	if err == nil {
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
						localGpgFiles = append(localGpgFiles, gpgReference{match[1], false})
					}
				}
				urls := urlmatch.FindAllString(strings.TrimPrefix(line, "gpgkey="), -1)
				for _, url := range urls {
					localGpgFiles = append(localGpgFiles, gpgReference{url, true})
				}
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
							localGpgFiles = append(localGpgFiles, gpgReference{match[1], false})
						}
					}
					urls := urlmatch.FindAllString(strings.TrimSpace(cont), -1)
					for _, url := range urls {
						localGpgFiles = append(localGpgFiles, gpgReference{url, true})
					}

				}
			}
			// save last values
			if localGpgCheck {
				gpgFiles = append(gpgFiles, localGpgFiles...)
			}
		}
	}
	return gpgCheck, gpgFiles

}
