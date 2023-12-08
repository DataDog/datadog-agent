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
)

const (
	packageConfig  = "/etc/dpkg/dpkg.cfg"
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

// getNoDebsig returns the signature policy for the host. no-debsig means GPG check is enabled
func getNoDebsig() bool {
	if _, err := os.Stat(packageConfig); !os.IsNotExist(err) {
		if file, err := os.Open(packageConfig); err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				if scanner.Text() == "no-debsig" {
					return true
				}
			}
		}
	}
	return false
}

// getAPTSignatureKeys returns the list of debian signature keys
func getAPTSignatureKeys(client *http.Client) []SigningKey {
	allKeys := make(map[string]SigningKey)
	// debian 11 and ubuntu 22.04 will be the last using legacy trusted.gpg.d folder and trusted.gpg file
	updateWithTrustedKeys(allKeys, client)
	// Regular files are referenced in the sources.list file by signed-by=filename
	updateWithSignedByKeys(allKeys, client)
	// In APT we can also sign packages with debsig
	keyPaths := getDebsigKeyPaths()
	for _, keyPath := range keyPaths {
		decryptGPGFile(allKeys, repoFile{filename: keyPath, repositories: nil}, "debsig", client)
	}
	// Extract SigningKeys in a list for the inventory
	var keyList []SigningKey
	for _, key := range allKeys {
		keyList = append(keyList, key)
	}
	return keyList
}

func updateWithTrustedKeys(allkeys map[string]SigningKey, client *http.Client) {
	// debian 11 and ubuntu 22.04 will be the last using legacy trusted.gpg.d folder and trusted.gpg file
	if _, err := os.Stat(trustedFolder); !os.IsNotExist(err) {
		if files, err := os.ReadDir(trustedFolder); err == nil {
			for _, file := range files {
				decryptGPGFile(allkeys, repoFile{filepath.Join(trustedFolder, file.Name()), nil}, "trusted", client)
			}
		}
	}
	if _, err := os.Stat(trustedFile); !os.IsNotExist(err) {
		decryptGPGFile(allkeys, repoFile{trustedFile, nil}, "trusted", client)
	}
}

func updateWithSignedByKeys(allKeys map[string]SigningKey, client *http.Client) {
	if _, err := os.Stat(mainSourceList); !os.IsNotExist(err) {
		reposPerKey := parseSourceListFile(mainSourceList)
		for name, repos := range reposPerKey {
			decryptGPGFile(allKeys, repoFile{name, repos}, "signed-by", client)
		}
	}
	if _, err := os.Stat(sourceList); !os.IsNotExist(err) {
		if files, err := os.ReadDir(sourceList); err == nil {
			for _, file := range files {
				reposPerKey := parseSourceListFile(filepath.Join(sourceList, file.Name()))
				for name, repos := range reposPerKey {
					decryptGPGFile(allKeys, repoFile{name, repos}, "signed-by", client)
				}
			}
		}
	}
}

func parseSourceListFile(filePath string) map[string][]repositories {
	reposPerKey := make(map[string][]repositories)
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		splitLine := sourceListRegexp.FindStringSubmatch(scanner.Text())
		if len(splitLine) > 1 {
			options := splitLine[2]
			keyURI := signedBy.FindStringSubmatch(options)
			if len(keyURI) > 1 {
				if _, ok := reposPerKey[keyURI[1]]; !ok {
					reposPerKey[keyURI[1]] = make([]repositories, 1)
					reposPerKey[keyURI[1]][0] = repositories{RepoName: strings.ReplaceAll(splitLine[3], " ", "/")}
				} else {
					reposPerKey[keyURI[1]] = append(reposPerKey[keyURI[1]], repositories{RepoName: strings.ReplaceAll(splitLine[3], " ", "/")})
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
