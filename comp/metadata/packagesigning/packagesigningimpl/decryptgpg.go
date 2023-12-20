// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagesigningimpl

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgUtils "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/utils"
	pgp "github.com/ProtonMail/go-crypto/openpgp"
)

// signingKey represents relevant fields for a package signature key
type signingKey struct {
	Fingerprint    string                `json:"signing_key_fingerprint"`
	ExpirationDate string                `json:"signing_key_expiration_date"`
	KeyType        string                `json:"signing_key_type"`
	Repositories   []pkgUtils.Repository `json:"repositories"`
}

type repoFile struct {
	filename     string
	repositories []pkgUtils.Repository
}

const (
	noExpDate  = "9999-12-31"
	formatDate = "2006-01-02"
)

// decryptGPGFile parse a gpg file (local or http) and extract signing keys information
// Some files can contain a list of repositories.
func decryptGPGFile(cacheKeys map[string]signingKey, gpgFile repoFile, keyType string, client *http.Client, logger log.Component) {
	var reader io.Reader
	if strings.HasPrefix(gpgFile.filename, "http") {
		response, err := client.Get(gpgFile.filename)
		if err != nil {
			return
		}
		defer response.Body.Close()
		reader = response.Body
	} else {
		file, err := os.Open(strings.Replace(gpgFile.filename, "file://", "", 1))
		if err != nil {
			return
		}
		defer file.Close()
		reader = file
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		return
	}
	decryptGPGContent(cacheKeys, content, keyType, gpgFile.repositories)
}

// decryptGPGContent extract keys from a byte slice (direct usage when reading from rpm db)
func decryptGPGContent(cacheKeys map[string]signingKey, content []byte, keyType string, repositories []pkgUtils.Repository) error {
	keyList, err := pgp.ReadKeyRing(bytes.NewReader(content))
	if err != nil {
		keyList, err = pgp.ReadArmoredKeyRing(bytes.NewReader(content))
		if err != nil {
			return err
		}
	}
	for _, key := range keyList {
		fingerprint := key.PrimaryKey.KeyIdString()
		i := key.PrimaryIdentity()
		keyLifetime := i.SelfSignature.KeyLifetimeSecs
		insertKey(cacheKeys, fingerprint, key.PrimaryKey.CreationTime, keyLifetime, keyType, repositories)
		// Insert also subkeys
		for _, subkey := range key.Subkeys {
			fingerprint = subkey.PublicKey.KeyIdString()
			keyLifetime = subkey.Sig.KeyLifetimeSecs
			insertKey(cacheKeys, fingerprint, subkey.PublicKey.CreationTime, keyLifetime, keyType, repositories)
		}
	}
	return nil
}

// insertKey will manage addition in the cacheKeys map: create a new entry or update an existing one (repositories part)
func insertKey(cacheKeys map[string]signingKey, fingerprint string, keyCreationTime time.Time, keyLifetime *uint32, keyType string, repositories []pkgUtils.Repository) {
	expDate := noExpDate
	if keyLifetime != nil {
		expiry := keyCreationTime.Add(time.Duration(*keyLifetime) * time.Second)
		expDate = expiry.Format(formatDate)
	}
	// We don't want to merge fingerprints when they exist with different key types
	index := fingerprint + keyType
	if currentKey, ok := cacheKeys[index]; ok {
		currentKey.Repositories = mergeRepositoryLists(currentKey.Repositories, repositories)
		cacheKeys[index] = currentKey
	} else {
		cacheKeys[index] = signingKey{
			Fingerprint:    fingerprint,
			ExpirationDate: expDate,
			KeyType:        keyType,
			Repositories:   repositories,
		}
	}
}

// mergeRepositoryList merge 2 lists of repositories and remove duplicates
func mergeRepositoryLists(a, b []pkgUtils.Repository) []pkgUtils.Repository {
	uniqueRepositories := make(map[string]struct{})
	for _, repo := range a {
		uniqueRepositories[repo.Name] = struct{}{}
	}
	for _, repo := range b {
		uniqueRepositories[repo.Name] = struct{}{}
	}
	mergedList := make([]pkgUtils.Repository, 0, len(uniqueRepositories))
	for repo := range uniqueRepositories {
		mergedList = append(mergedList, pkgUtils.Repository{Name: repo})
	}
	return mergedList
}
