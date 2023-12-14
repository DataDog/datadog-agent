// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagesigningimpl

import (
	"bufio"
	"encoding/xml"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgUtils "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/utils"
)

const (
	trustedFolder  = "/etc/apt/trusted.gpg.d/"
	trustedFile    = "/etc/apt/trusted.gpg"
	mainSourceList = "/etc/apt/sources.list"
	sourceList     = "/etc/apt/sources.list.d/"
)

var (
	sourceListRegexp = regexp.MustCompile(`^([^\s]+)\s?(\[.*\]\s)?(.*)$`)
	signedBy         = regexp.MustCompile(`signed-by=([A-Za-z0-9_\-\.\/]+)`)
	debsigPolicies   = "/etc/debsig/policies/"
	debsigKeyring    = "/usr/share/debsig/keyrings/"
)

// getAPTSignatureKeys returns the list of debian signature keys
func getAPTSignatureKeys(client *http.Client, logger log.Component) []SigningKey {
	allKeys := make(map[string]SigningKey)
	// debian 11 and ubuntu 22.04 will be the last using legacy trusted.gpg.d folder and trusted.gpg file
	updateWithTrustedKeys(allKeys, client, logger)
	// Regular files are referenced in the sources.list file by signed-by=filename
	updateWithSignedByKeys(allKeys, client, logger)
	// In APT we can also sign packages with debsig
	keyPaths := getDebsigKeyPaths()
	for _, keyPath := range keyPaths {
		decryptGPGFile(allKeys, repoFile{filename: keyPath, repositories: nil}, "debsig", client, logger)
	}
	// Extract SigningKeys in a list for the inventory
	var keyList []SigningKey
	for _, key := range allKeys {
		keyList = append(keyList, key)
	}
	return keyList
}

func updateWithTrustedKeys(allKeys map[string]SigningKey, client *http.Client, logger log.Component) {
	// debian 11 and ubuntu 22.04 will be the last using legacy trusted.gpg.d folder and trusted.gpg file
	if _, err := os.Stat(trustedFolder); !os.IsNotExist(err) {
		if files, err := os.ReadDir(trustedFolder); err == nil {
			for _, file := range files {
				trustedFileName := filepath.Join(trustedFolder, file.Name())
				decryptGPGFile(allKeys, repoFile{trustedFileName, nil}, "trusted", client, logger)
			}
		}
	}
	if _, err := os.Stat(trustedFile); !os.IsNotExist(err) {
		decryptGPGFile(allKeys, repoFile{trustedFile, nil}, "trusted", client, logger)
	}
}

func updateWithSignedByKeys(allKeys map[string]SigningKey, client *http.Client, logger log.Component) {
	if _, err := os.Stat(mainSourceList); !os.IsNotExist(err) {
		reposPerKey := parseSourceListFile(mainSourceList)
		for name, repos := range reposPerKey {
			decryptGPGFile(allKeys, repoFile{name, repos}, "signed-by", client, logger)
		}
	}
	if _, err := os.Stat(sourceList); !os.IsNotExist(err) {
		if files, err := os.ReadDir(sourceList); err == nil {
			for _, file := range files {
				reposPerKey := parseSourceListFile(filepath.Join(sourceList, file.Name()))
				for name, repos := range reposPerKey {
					decryptGPGFile(allKeys, repoFile{name, repos}, "signed-by", client, logger)
				}
			}
		}
	}
}

func parseSourceListFile(filePath string) map[string][]pkgUtils.Repositories {
	reposPerKey := make(map[string][]pkgUtils.Repositories)
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		splitLine := sourceListRegexp.FindStringSubmatch(line)
		if len(splitLine) > 1 {
			options := splitLine[2]
			keyURI := signedBy.FindStringSubmatch(options)
			if len(keyURI) > 1 {
				if _, ok := reposPerKey[keyURI[1]]; !ok {
					reposPerKey[keyURI[1]] = []pkgUtils.Repositories{{RepoName: strings.ReplaceAll(splitLine[3], " ", "/")}}
				} else {
					reposPerKey[keyURI[1]] = append(reposPerKey[keyURI[1]], pkgUtils.Repositories{RepoName: strings.ReplaceAll(splitLine[3], " ", "/")})
				}
			}
		}
	}
	return reposPerKey
}

func getDebsigKeyPaths() []string {
	filePaths := make(map[string]struct{})
	// Search in the policy files
	if _, err := os.Stat(debsigPolicies); !os.IsNotExist(err) {
		if debsigDirs, err := os.ReadDir(debsigPolicies); err == nil {
			for _, debsigDir := range debsigDirs {
				if debsigDir.IsDir() {
					if policyFiles, err := os.ReadDir(filepath.Join(debsigPolicies, debsigDir.Name())); err == nil {
						for _, policyFile := range policyFiles {
							// Get the gpg file name from policy files
							if debsigFile := getDebsigFileFromPolicy(filepath.Join(debsigPolicies, debsigDir.Name(), policyFile.Name())); debsigFile != "" {
								debsigFilePath := filepath.Join(debsigKeyring, debsigDir.Name(), debsigFile)
								if _, err := os.Stat(debsigFilePath); !os.IsNotExist(err) {
									filePaths[debsigFilePath] = struct{}{}
								}
							}
						}
					}
				}
			}
		}
	}
	// Denormalise the map
	filePathsSlice := make([]string, 0)
	for k := range filePaths {
		filePathsSlice = append(filePathsSlice, k)
	}
	return filePathsSlice
}

// policy structure to unmarshall the policy files
type policy struct {
	XMLName      xml.Name `xml:"Policy"`
	Origin       origin   `xml:"Origin"`
	Selection    selection
	Verification verification
}

type origin struct {
	Name        string `xml:"Name,attr"`
	ID          string `xml:"id,attr"`
	Description string `xml:"Description,attr"`
}

type selection struct {
	Required required `xml:"Required"`
}

type required struct {
	Type string `xml:"Type,attr"`
	File string `xml:"File,attr"`
	ID   string `xml:"id,attr"`
}

type verification struct {
	MinOptional int      `xml:"MinOptional,attr"`
	Required    required `xml:"Required"`
}

func getDebsigFileFromPolicy(policyFile string) string {
	if xmlData, err := os.ReadFile(policyFile); err == nil {
		var policy policy
		if err = xml.Unmarshal(xmlData, &policy); err == nil {
			return policy.Verification.Required.File
		}
	}
	return ""
}
