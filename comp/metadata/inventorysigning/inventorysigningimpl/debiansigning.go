// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorysigningimpl

import (
	"bufio"
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
)

// GetDebianSignatureKeys returns the list of debian signature keys
func GetDebianSignatureKeys() []SigningKey {
	// debian 11 and ubuntu 22.04 will be the last using legacy trusted.gpg.d folder and trusted.gpg file
	allKeys := getTrustedKeys()
	// Regular files are referenced in the sources.list file by signed-by=filename
	allKeys = append(allKeys, getKeysFromGPGFiles(getSignedByFiles(), "", "signed-by")...)
	// In APT we can also sign packages with debsig
	allKeys = append(allKeys, getDebsigKeys()...)
	return allKeys
}

func getTrustedKeys() []SigningKey {
	var allKeys []SigningKey
	// debian 11 and ubuntu 22.04 will be the last using legacy trusted.gpg.d folder and trusted.gpg file
	trustedFolder := "/etc/apt/trusted.gpg.d/"
	if _, err := os.Stat(trustedFolder); os.IsExist(err) {
		if files, err := os.ReadDir(trustedFolder); err == nil {
			allKeys = append(allKeys, getKeysFromGPGFiles(files, trustedFolder, "trusted")...)
		}
	}
	var trustedFiles []os.DirEntry
	trustedFiles = append(trustedFiles, fileEntry{name: "/etc/apt/trusted.gpg"})
	if _, err := os.Stat(trustedFiles[0].Name()); os.IsExist(err) {
		allKeys = append(allKeys, getKeysFromGPGFiles(trustedFiles, "", "trusted")...)
	}
	return allKeys
}

func getSignedByFiles() []os.DirEntry {
	var signedByFiles []os.DirEntry
	signedByMatch := regexp.MustCompile(`signed-by=([A-Za-z0-9_\-\.\/]+)`)
	mainSourceList := "/etc/apt/sources.list"
	if _, err := os.Stat(mainSourceList); os.IsExist(err) {
		signedByFiles = append(signedByFiles, extractPattern(mainSourceList, signedByMatch)...)
	}
	sourceList := "/etc/apt/sources.list.d/"
	if _, err := os.Stat(sourceList); os.IsExist(err) {
		if files, err := os.ReadDir(sourceList); err == nil {
			for _, file := range files {
				signedByFiles = append(signedByFiles, extractPattern(filepath.Join(sourceList, file.Name()), signedByMatch)...)
			}
		}
	}
	return signedByFiles
}

func getDebsigKeys() []SigningKey {
	var allKeys []SigningKey
	debsigPolicies := "/etc/debsig/policies/"
	debsigKeyring := "/usr/share/debsig/keyrings/"
	// Search in the policy files
	if _, err := os.Stat(debsigPolicies); os.IsExist(err) {
		if files, err := os.ReadDir(debsigPolicies); err == nil {
			for _, file := range files {
				if file.IsDir() {
					if policyFiles, err := os.ReadDir(debsigPolicies + file.Name()); err != nil {
						for _, policyFile := range policyFiles {
							// Get the gpg file name from policy files
							if debsigFile, _ := getDebsigFileFromPolicy(filepath.Join(debsigPolicies, file.Name(), policyFile.Name())); debsigFile != "" {
								debsigFilePath := filepath.Join(debsigKeyring, file.Name(), debsigFile)
								if _, err := os.Stat(debsigFilePath); os.IsExist(err) {
									allKeys = append(allKeys, getKeysFromGPGFiles([]os.DirEntry{fileEntry{name: debsigFilePath}}, "", "debsig")...)
								}
							}
						}
					}
				}
			}
		}
	}
	return allKeys
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

func getDebsigFileFromPolicy(policyFile string) (string, error) {
	if xmlData, err := os.ReadFile(policyFile); err == nil {
		var policy policy
		if err = xml.Unmarshal(xmlData, &policy); err == nil {
			return policy.Verification.Required.File, nil
		}
	}
	return "", nil
}
