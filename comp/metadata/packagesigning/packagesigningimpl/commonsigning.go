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

	pgp "github.com/ProtonMail/go-crypto/openpgp"
)

// SigningKey represents relevant fields for a package signature key
type SigningKey struct {
	Fingerprint    string `json:"signing_key_fingerprint"`
	ExpirationDate string `json:"signing_key_expiration_date"`
	KeyType        string `json:"signing_key_type"`
	// Repositories   []repositories `json:"repositories"`
}

// type repositories struct {
// 	RepoName string `json:"repo_name"`
// }

const (
	aptPath    = "/etc/apt"
	yumPath    = "/etc/yum"
	dnfPath    = "/etc/dnf"
	zyppPath   = "/etc/zypp"
	noExpDate  = "9999-12-31"
	formatDate = "2006-01-02"
)

// getPackageManager is a lazy implementation to detect if we use APT or YUM (RH or SUSE)
func getPackageManager() string {
	if _, err := os.Stat(aptPath); os.IsExist(err) {
		return "apt"
	} else if _, err := os.Stat(yumPath); os.IsExist(err) {
		return "yum"
	} else if _, err := os.Stat(dnfPath); os.IsExist(err) {
		return "dnf"
	} else if _, err := os.Stat(zyppPath); os.IsExist(err) {
		return "zypper"
	}
	return ""
}

// decryptGPGFile parse a gpg file (local or http) and extract signing keys information
func decryptGPGFile(allKeys map[SigningKey]struct{}, gpgFile string, keyType string) {
	var reader io.Reader
	if strings.HasPrefix(gpgFile, "http") {
		response, err := http.Get(gpgFile)
		if err != nil {
			return
		}
		defer response.Body.Close()
		reader = response.Body
	} else {
		file, err := os.Open(gpgFile)
		if err != nil {
			return
		}
		defer file.Close()
		reader = file
	}
	decryptGPGReader(allKeys, reader, keyType)
}

// decryptGPGReader extract keys from a reader, useful for rpm extraction
func decryptGPGReader(allKeys map[SigningKey]struct{}, reader io.Reader, keyType string) {
	keyList, err := pgp.ReadArmoredKeyRing(reader)
	if err != nil {
		return
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
		allKeys[SigningKey{
			Fingerprint:    fingerprint,
			ExpirationDate: expDate,
			KeyType:        keyType,
		}] = struct{}{}
	}
}
