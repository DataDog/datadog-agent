// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package util

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"testing"
)

// The following certificate and key are used for testing purposes only.
// They have been generated using the following command:
//
//	openssl req -x509 -newkey ec:<(openssl ecparam -name prime256v1) -keyout key.pem -out cert.pem -days 3650 \
//	  -subj "/O=Datadog, Inc." \
//	  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
//	  -addext "keyUsage=digitalSignature,keyEncipherment" \
//	  -addext "extendedKeyUsage=serverAuth,clientAuth" \
//	  -addext "basicConstraints=CA:FALSE" \
//	  -nodes
var (
	testIPCCert = []byte(`-----BEGIN CERTIFICATE-----
MIIByjCCAW+gAwIBAgIUFMz01UTXNav1uXY4h88qaN2PVXowCgYIKoZIzj0EAwIw
GDEWMBQGA1UECgwNRGF0YWRvZywgSW5jLjAeFw0yNTAzMjExNjEzNDVaFw0zNTAz
MTkxNjEzNDVaMBgxFjAUBgNVBAoMDURhdGFkb2csIEluYy4wWTATBgcqhkjOPQIB
BggqhkjOPQMBBwNCAAR/R+iIKhns7nVF0LcMtHcRcviPiC7DB5jJPF5DkIUPE8Lj
uCOOCdJC3pJrv05NmkvBKuQJMqkv07bG8KR6QIERo4GWMIGTMB0GA1UdDgQWBBSH
GJDH6ta1dchIa+jz/ToUKyiKXjAfBgNVHSMEGDAWgBSHGJDH6ta1dchIa+jz/ToU
KyiKXjAaBgNVHREEEzARgglsb2NhbGhvc3SHBH8AAAEwCwYDVR0PBAQDAgWgMB0G
A1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjAJBgNVHRMEAjAAMAoGCCqGSM49
BAMCA0kAMEYCIQDj7Q0twsRygmWRcUZLD/ztXcQh8pZPjeAyTVETzafrngIhAMxE
GqfvQ4TyJibfnZEMwY0DYqM4YsvvhxAPZbU01eNa
-----END CERTIFICATE-----
`)
	testIPCKey = []byte(`-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgXdthAqhgD8qpW9wj
2Md4KA1q1eml9YehNB4SCg/hcsKhRANCAAR/R+iIKhns7nVF0LcMtHcRcviPiC7D
B5jJPF5DkIUPE8LjuCOOCdJC3pJrv05NmkvBKuQJMqkv07bG8KR6QIER
-----END PRIVATE KEY-----
`)
)

// SetAuthTokenInMemory is only expected to be used for unit-tests
// It sets the auth token, client TLS config and server TLS config in memory
// and initializes the initSource to setAuthTokenInMemory
func SetAuthTokenInMemory(t testing.TB) {
	tokenLock.Lock()
	defer tokenLock.Unlock()

	if initSource != uninitialized {
		if initSource != setAuthTokenInMemory {
			t.Fatal("the auth stack have been initialized by un underlying part of the code")
		}
		t.Log("the auth stack have been initialized in a previous call to SetAuthTokenInMemory, no need to generate new values")
		return
	}

	t.Log("generating random token, clientTLSConfig and serverTLSConfig for test")

	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("can't create agent auth token value: %v", err)
	}

	// convert the raw token to an hex string
	token = hex.EncodeToString(key)

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(testIPCCert); !ok {
		t.Fatalf("Unable to generate certPool from PERM IPC cert")
	}

	clientTLSConfig = &tls.Config{
		RootCAs: certPool,
	}

	tlsCert, err := tls.X509KeyPair(testIPCCert, testIPCKey)
	if err != nil {
		t.Fatalf("Unable to generate x509 cert from PERM IPC cert and key: %v", err)
	}
	serverTLSConfig = &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}

	initSource = setAuthTokenInMemory
}
