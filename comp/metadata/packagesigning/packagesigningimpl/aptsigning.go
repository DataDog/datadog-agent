// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagesigningimpl

import (
	"bufio"
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
)

const (
	packageConfig  = "/etc/dpkg/dpkg.cfg"
	trustedFolder  = "/etc/apt/trusted.gpg.d/"
	trustedFile    = "/etc/apt/trusted.gpg"
	mainSourceList = "/etc/apt/sources.list"
	sourceList     = "/etc/apt/sources.list.d/"
	debsigPolicies = "/etc/debsig/policies/"
	debsigKeyring  = "/usr/share/debsig/keyrings/"
)

// getNoDebsig returns the signature policy for the host. no-debsig means GPG check is enabled
func getNoDebsig() bool {
	if _, err := os.Stat(packageConfig); os.IsExist(err) {
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
func getAPTSignatureKeys() []SigningKey {
	allKeys := make(map[SigningKey]struct{})
	// debian 11 and ubuntu 22.04 will be the last using legacy trusted.gpg.d folder and trusted.gpg file
	getTrustedKeys(allKeys)
	// Regular files are referenced in the sources.list file by signed-by=filename
	for _, file := range getSignedByFiles() {
		decryptGPGFile(allKeys, file.Name(), "signed-by")
	}
	// In APT we can also sign packages with debsig
	getDebsigKeys(allKeys)
	var keyList []SigningKey
	for keys := range allKeys {
		keyList = append(keyList, keys)
	}
	return keyList
}

func getTrustedKeys(allkeys map[SigningKey]struct{}) {
	// debian 11 and ubuntu 22.04 will be the last using legacy trusted.gpg.d folder and trusted.gpg file
	if _, err := os.Stat(trustedFolder); os.IsExist(err) {
		if files, err := os.ReadDir(trustedFolder); err == nil {
			for _, file := range files {
				decryptGPGFile(allkeys, filepath.Join(trustedFolder, file.Name()), "trusted")
			}
		}
	}
	if _, err := os.Stat(trustedFile); os.IsExist(err) {
		decryptGPGFile(allkeys, trustedFile, "trusted")
	}
}

func getSignedByFiles() []os.DirEntry {
	var signedByFiles []os.DirEntry
	signedByMatch := regexp.MustCompile(`signed-by=([A-Za-z0-9_\-\.\/]+)`)
	if _, err := os.Stat(mainSourceList); os.IsExist(err) {
		signedByFiles = append(signedByFiles, extractPattern(mainSourceList, signedByMatch)...)
	}
	if _, err := os.Stat(sourceList); os.IsExist(err) {
		if files, err := os.ReadDir(sourceList); err == nil {
			for _, file := range files {
				signedByFiles = append(signedByFiles, extractPattern(filepath.Join(sourceList, file.Name()), signedByMatch)...)
			}
		}
	}
	return signedByFiles
}

func getDebsigKeys(allKeys map[SigningKey]struct{}) {
	// Search in the policy files
	if _, err := os.Stat(debsigPolicies); os.IsExist(err) {
		if debsigDirs, err := os.ReadDir(debsigPolicies); err == nil {
			for _, debsigDir := range debsigDirs {
				if debsigDir.IsDir() {
					if policyFiles, err := os.ReadDir(filepath.Join(debsigPolicies, debsigDir.Name())); err == nil {
						for _, policyFile := range policyFiles {
							// Get the gpg file name from policy files
							if debsigFile := getDebsigFileFromPolicy(filepath.Join(debsigPolicies, debsigDir.Name(), policyFile.Name())); debsigFile != "" {
								debsigFilePath := filepath.Join(debsigKeyring, debsigDir.Name(), debsigFile)
								if _, err := os.Stat(debsigFilePath); os.IsExist(err) {
									decryptGPGFile(allKeys, debsigFilePath, "debsig")
								}
							}
						}
					}
				}
			}
		}
	}
}

// fileEntry implements the os.DirEntry interface to generate a list of files from string
type fileEntry struct {
	name string
}

func (fe fileEntry) Name() string {
	return fe.name
}
func (fe fileEntry) IsDir() bool {
	return false
}
func (fe fileEntry) Type() os.FileMode {
	return 0
}
func (fe fileEntry) Info() (os.FileInfo, error) {
	return nil, nil
}

func extractPattern(filePath string, pattern *regexp.Regexp) []os.DirEntry {
	var patterns []os.DirEntry
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		matchedFiles := pattern.FindAllStringSubmatch(scanner.Text(), -1)
		for _, matchedFile := range matchedFiles {
			if len(matchedFile) > 1 {
				patterns = append(patterns, fileEntry{name: matchedFile[1]})
			}
		}
	}
	return patterns
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
