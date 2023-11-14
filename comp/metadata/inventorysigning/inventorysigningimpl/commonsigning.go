// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorysigningimpl

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// SigningKey represents relevant fields for a package signature key
type SigningKey struct {
	Fingerprint    string `json:"signing_key_fingerprint"`
	ExpirationDate string `json:"signing_key_expiration_date"`
	KeyType        string `json:"signing_key_type"`
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

func getKeysFromGPGFiles(files []os.DirEntry, parent string, keyType string) []SigningKey {
	var keys []SigningKey
	for _, file := range files {
		keys = decryptGPGFile("gpg "+filepath.Join(parent, file.Name()), keyType)

	}
	return keys
}

func decryptGPGFile(gpgCommand string, keyType string) []SigningKey {
	cmd := exec.Command("bash", "-c", gpgCommand)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Create regular expressions for date and key ID extraction
	dateRegex := regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}`)
	keyIDRegex := regexp.MustCompile(`[A-Z0-9]+`)

	var keys []SigningKey
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "pub") {
			expDate := "9999-12-31"
			dateList := dateRegex.FindAllString(line, -1)
			if len(dateList) > 1 {
				expDate = dateList[1]
			}
			// Read the next line to extract the key ID
			if scanner.Scan() {
				keyID := keyIDRegex.FindString(scanner.Text())
				keys = append(keys, SigningKey{
					Fingerprint:    keyID,
					ExpirationDate: expDate,
					KeyType:        keyType,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil
	}
	return keys
}
