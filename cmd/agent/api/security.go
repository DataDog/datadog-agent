package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type contextKey int

const (
	contextKeyTokenInfoID contextKey = iota
)

var (
	tlsKeyPair  *tls.Certificate
	tlsCertPool *x509.CertPool
	tlsAddr     string
)

// validateToken - validates token for legacy API
func validateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := util.Validate(w, r); err != nil {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// parseToken parses the token and validate it for our gRPC API, it returns an empty
// struct and an error or nil
func parseToken(token string) (struct{}, error) {
	if token != util.GetAuthToken() {
		return struct{}{}, errors.New("Invalid session token")
	}

	// Currently this empty struct doesn't add any information
	// to the context, but we could potentially add some custom
	// type.
	return struct{}{}, nil
}

//grpcAuthFunc is a middleware (interceptor) that extracts and verifies token from header
func grpcAuth(ctx context.Context) (context.Context, error) {

	token, err := grpc_auth.AuthFromMD(ctx, "Bearer")
	if err != nil {
		return nil, err
	}

	tokenInfo, err := parseToken(token)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid auth token: %v", err)
	}

	// do we need this at all?
	newCtx := context.WithValue(ctx, contextKeyTokenInfoID, tokenInfo)

	return newCtx, nil
}

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
