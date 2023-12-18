// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagesigningimpl

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgUtils "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/utils"
	pgp "github.com/ProtonMail/go-crypto/openpgp"
)

// SigningKey represents relevant fields for a package signature key
type SigningKey struct {
	Fingerprint    string                  `json:"signing_key_fingerprint"`
	ExpirationDate string                  `json:"signing_key_expiration_date"`
	KeyType        string                  `json:"signing_key_type"`
	Repositories   []pkgUtils.Repositories `json:"repositories"`
}

type repoFile struct {
	filename     string
	repositories []pkgUtils.Repositories
}

const (
	noExpDate  = "9999-12-31"
	formatDate = "2006-01-02"
)

// decryptGPGFile parse a gpg file (local or http) and extract signing keys information
// Some files can contain a list of repositories.
// It is called twice to parse the keyring as armored keyring or not
func decryptGPGFile(allKeys map[string]SigningKey, gpgFile repoFile, keyType string, client *http.Client, logger log.Component) {
	err := decrypt(allKeys, gpgFile, false, keyType, client)
	if err != nil {
		err = decrypt(allKeys, gpgFile, true, keyType, client)
		if err != nil {
			logger.Infof("Error while parsing gpg file %s: %s", gpgFile.filename, err)
		}
	}
}

// decrypt is an intermediate method which opens a content (io.Reader) and calls the real parser
func decrypt(allKeys map[string]SigningKey, gpgFile repoFile, armored bool, keyType string, client *http.Client) error {
	var reader io.Reader
	if strings.HasPrefix(gpgFile.filename, "http") {
		response, err := client.Get(gpgFile.filename)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		reader = response.Body
	} else {
		file, err := os.Open(strings.Replace(gpgFile.filename, "file://", "", 1))
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
	}
	return decryptGPGReader(allKeys, reader, armored, keyType, gpgFile.repositories)
}

// decryptGPGReader extract keys from a reader, useful for rpm extraction
func decryptGPGReader(allKeys map[string]SigningKey, reader io.Reader, armored bool, keyType string, repositories []pkgUtils.Repositories) error {
	pgpReader := pgp.ReadKeyRing
	if armored {
		pgpReader = pgp.ReadArmoredKeyRing
	}
	keyList, err := pgpReader(reader)
	if err != nil {
		return err
	}
	for _, key := range keyList {
		fingerprint := key.PrimaryKey.KeyIdString()
		expDate := noExpDate
		i := key.PrimaryIdentity()
		keyLifetime := i.SelfSignature.KeyLifetimeSecs
		if keyLifetime != nil {
			expiry := key.PrimaryKey.CreationTime.Add(time.Duration(*i.SelfSignature.KeyLifetimeSecs) * time.Second)
			expDate = expiry.Format(formatDate)
		}
		if _, ok := allKeys[fingerprint]; ok {
			currentKey := allKeys[fingerprint]
			currentKey.Repositories = mergeRepositoryLists(currentKey.Repositories, repositories)
			allKeys[fingerprint] = currentKey
		} else {
			allKeys[fingerprint] = SigningKey{
				Fingerprint:    fingerprint,
				ExpirationDate: expDate,
				KeyType:        keyType,
				Repositories:   repositories,
			}
		}
	}
	return nil
}

// mergeRepositoryList merge 2 lists of repositories and remove duplicates
func mergeRepositoryLists(a, b []pkgUtils.Repositories) []pkgUtils.Repositories {
	uniqueRepositories := make(map[string]struct{})
	for _, repo := range a {
		uniqueRepositories[repo.RepoName] = struct{}{}
	}
	for _, repo := range b {
		uniqueRepositories[repo.RepoName] = struct{}{}
	}
	mergedList := make([]pkgUtils.Repositories, 0, len(uniqueRepositories))
	for repo := range uniqueRepositories {
		mergedList = append(mergedList, pkgUtils.Repositories{RepoName: repo})
	}
	return mergedList
}
