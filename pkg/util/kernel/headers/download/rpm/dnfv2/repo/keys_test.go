// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package repo

import (
	"bytes"
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/stretchr/testify/require"
)

func signingKey(t *testing.T) (*openpgp.Entity, []byte) {
	t.Helper()
	key, err := openpgp.NewEntity("header test", "", "test@example.invalid", &packet.Config{RSABits: 2048})
	require.NoError(t, err)
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	require.NoError(t, err)
	require.NoError(t, key.Serialize(w))
	require.NoError(t, w.Close())
	return key, buf.Bytes()
}

func TestSigningKeyTransport(t *testing.T) {
	_, key := signingKey(t)
	insecureRequests := 0
	insecure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { insecureRequests++; _, _ = w.Write(key) }))
	defer insecure.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, insecure.URL, http.StatusFound)
			return
		}
		if r.URL.Path == "/loop" {
			http.Redirect(w, r, "/loop", http.StatusFound)
			return
		}
		_, _ = w.Write(key)
	}))
	defer secure.Close()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.asc")
	require.NoError(t, os.WriteFile(keyPath, key, 0600))
	caPath := filepath.Join(dir, "ca.pem")
	require.NoError(t, os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: secure.Certificate().Raw}), 0600))
	for _, tc := range []struct {
		name, url, ca string
		ok            bool
	}{
		{"local key", "file://" + keyPath, "", true},
		{"HTTP key", insecure.URL, "", false},
		{"untrusted certificate", secure.URL, "", false},
		{"custom CA", secure.URL, caPath, true},
		{"downgrade redirect", secure.URL + "/redirect", caPath, false},
		{"redirect loop", secure.URL + "/loop", caPath, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// sslverify=0 for packages must never disable signing-key TLS verification.
			repo := Repo{SSLVerify: false, SSLCaCert: tc.ca}
			client, err := repo.httpClient(verifiedHTTPSTransport)
			require.NoError(t, err)
			keys, err := readGPGKey(context.Background(), client, tc.url)
			if tc.ok {
				require.NoError(t, err)
				require.Len(t, keys, 1)
			} else {
				require.Error(t, err)
			}
		})
	}
	require.Zero(t, insecureRequests, "neither HTTP keys nor downgraded redirects may be fetched")
	client, err := (&Repo{}).httpClient(verifiedHTTPSTransport)
	require.NoError(t, err)
	keys, err := readGPGKeys(context.Background(), client, []string{insecure.URL, "file://" + keyPath})
	require.Error(t, err)
	require.Len(t, keys, 1, "an unusable key source must not hide another trusted key")
}
