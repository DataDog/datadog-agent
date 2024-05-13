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
	trusted          = regexp.MustCompile(`trusted=yes`)
	debsigPolicies   = "/etc/debsig/policies/"
	debsigKeyring    = "/usr/share/debsig/keyrings/"
)

// getAPTSignatureKeys returns the list of debian signature keys
func getAPTSignatureKeys(client *http.Client, logger log.Component) []signingKey {
	cacheKeys := make(map[string]signingKey)
	// debian 11 and ubuntu 22.04 will be the last using legacy trusted.gpg.d folder and trusted.gpg file
	err := updateWithTrustedKeys(cacheKeys, client)
	if err != nil {
		logger.Debugf("Error while reading trusted keys: %s", err)
	}
	// Regular files are referenced in the sources.list file by signed-by=filename
	err = updateWithSignedByKeys(cacheKeys, client)
	if err != nil {
		logger.Debugf("Error while reading signed-by keys: %s", err)
	}
	// In APT we can also sign packages with debsig
	keyPaths, err := getDebsigKeyPaths()
	if err != nil {
		logger.Debugf("Error while reading debsig keys: %s", err)
	}
	for _, keyPath := range keyPaths {
		err = readGPGFile(cacheKeys, repoFile{filename: keyPath, repositories: nil}, "debsig", client)
		if err != nil {
			logger.Debugf("Error while reading debsig key %s: %s", keyPath, err)
			continue
		}
	}
	// Extract signingKeys from the cache in a list
	var keyList []signingKey
	for _, key := range cacheKeys {
		keyList = append(keyList, key)
	}
	return keyList
}

func updateWithTrustedKeys(cacheKeys map[string]signingKey, client *http.Client) error {
	// debian 11 and ubuntu 22.04 will be the last using legacy trusted.gpg.d folder and trusted.gpg file
	if _, err := os.Stat(trustedFolder); err != nil {
		return err
	}
	if files, err := os.ReadDir(trustedFolder); err == nil {
		for _, file := range files {
			trustedFileName := filepath.Join(trustedFolder, file.Name())
			err = readGPGFile(cacheKeys, repoFile{trustedFileName, nil}, "trusted", client)
			if err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(trustedFile); err != nil {
		return err
	}
	return readGPGFile(cacheKeys, repoFile{trustedFile, nil}, "trusted", client)
}

func updateWithSignedByKeys(cacheKeys map[string]signingKey, client *http.Client) error {
	gpgcheck, err := pkgUtils.IsPackageSigningEnabled()
	if err != nil {
		return err
	}
	if _, err := os.Stat(mainSourceList); err != nil {
		return err
	}
	reposPerKey := parseSourceListFile(mainSourceList, gpgcheck)
	for name, repos := range reposPerKey {
		err := readGPGFile(cacheKeys, repoFile{name, repos}, "signed-by", client)
		if err != nil {
			return err
		}
	}

	if _, err := os.Stat(sourceList); err != nil {
		return err
	}
	if files, err := os.ReadDir(sourceList); err == nil {
		for _, file := range files {
			reposPerKey := parseSourceListFile(filepath.Join(sourceList, file.Name()), gpgcheck)
			for name, repos := range reposPerKey {
				err = readGPGFile(cacheKeys, repoFile{name, repos}, "signed-by", client)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func parseSourceListFile(filePath string, gpgcheck bool) map[string][]pkgUtils.Repository {
	reposPerKey := make(map[string][]pkgUtils.Repository)
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
			repoGpgcheck := true
			repoCheck := trusted.FindStringSubmatch(options)
			if len(repoCheck) > 0 {
				repoGpgcheck = false
			}
			keyPath := "nokey" // Track repositories without gpgkey
			if len(keyURI) > 1 {
				keyPath = keyURI[1]
			}
			if _, ok := reposPerKey[keyPath]; !ok {
				reposPerKey[keyPath] = []pkgUtils.Repository{{Name: splitLine[3], Enabled: true, GPGCheck: gpgcheck, RepoGPGCheck: repoGpgcheck}}
			} else {
				reposPerKey[keyPath] = append(reposPerKey[keyPath], pkgUtils.Repository{Name: splitLine[3], Enabled: true, GPGCheck: gpgcheck, RepoGPGCheck: repoGpgcheck})
			}
		}
	}
	return reposPerKey
}

func getDebsigKeyPaths() ([]string, error) {
	filePaths := make(map[string]struct{})
	// Search in the policy files
	if _, err := os.Stat(debsigPolicies); err != nil {
		return nil, err
	}
	if debsigDirs, err := os.ReadDir(debsigPolicies); err == nil {
		for _, debsigDir := range debsigDirs {
			if debsigDir.IsDir() {
				if policyFiles, err := os.ReadDir(filepath.Join(debsigPolicies, debsigDir.Name())); err == nil {
					for _, policyFile := range policyFiles {
						// Get the gpg file name from policy files
						if debsigFile := getDebsigFileFromPolicy(filepath.Join(debsigPolicies, debsigDir.Name(), policyFile.Name())); debsigFile != "" {
							debsigFilePath := filepath.Join(debsigKeyring, debsigDir.Name(), debsigFile)
							if _, err := os.Stat(debsigFilePath); err == nil {
								filePaths[debsigFilePath] = struct{}{}
							} else {
								return nil, err
							}
						}
					}
				} else {
					return nil, err
				}
			}
		}
	} else {
		return nil, err
	}
	// Denormalise the map
	filePathsSlice := make([]string, 0)
	for k := range filePaths {
		filePathsSlice = append(filePathsSlice, k)
	}
	return filePathsSlice, nil
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
