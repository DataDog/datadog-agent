package api

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"

	"github.com/DataDog/datadog-agent/pkg/api/security"
)

var (
	tlsKeyPair  *tls.Certificate
	tlsCertPool *x509.CertPool
	tlsAddr     string
)

func buildSelfSignedKeyPair() ([]byte, []byte) {

	hosts := []string{"127.0.0.1", "localhost", "::1"}
	ipcAddr, err := getIPCAddressPort()
	if err == nil {
		hosts = append(hosts, ipcAddr)
	}
	_, rootCertPEM, rootKey, err := security.GenerateRootCert(hosts, 2048)
	if err != nil {
		return nil, nil
	}

	// PEM encode the private key
	rootKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey),
	})

	// Create and return TLS private cert and key
	return rootCertPEM, rootKeyPEM
}

func initializeTLS() {

	cert, key := buildSelfSignedKeyPair()
	if cert == nil {
		panic("unable to generate certificate")
	}
	pair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		panic(err)
	}
	tlsKeyPair = &pair
	tlsCertPool = x509.NewCertPool()
	ok := tlsCertPool.AppendCertsFromPEM(cert)
	if !ok {
		panic("bad certs")
	}

	tlsAddr, err = getIPCAddressPort()
	if err != nil {
		panic("unable to get IPC address and port")
	}
}
