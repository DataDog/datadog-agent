// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transport

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
)

// AuthType identifies the authentication method.
type AuthType string

const (
	// AuthTypeNone indicates no authentication.
	AuthTypeNone AuthType = "none"

	// AuthTypeMTLS indicates mutual TLS authentication.
	AuthTypeMTLS AuthType = "mtls"

	// AuthTypeToken indicates token-based authentication.
	AuthTypeToken AuthType = "token"
)

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	// Type is the authentication type.
	Type AuthType

	// Token is the expected token for token-based authentication.
	// Only used when Type is AuthTypeToken.
	Token string

	// AllowedCNs is a list of allowed Common Names for mTLS authentication.
	// If empty, any valid client certificate is accepted.
	// Only used when Type is AuthTypeMTLS.
	AllowedCNs []string

	// AllowedOrgs is a list of allowed Organizations for mTLS authentication.
	// If empty, any valid client certificate is accepted.
	// Only used when Type is AuthTypeMTLS.
	AllowedOrgs []string
}

// NoopAuthenticator accepts all connections without authentication.
type NoopAuthenticator struct{}

// NewNoopAuthenticator creates a new no-op authenticator.
func NewNoopAuthenticator() *NoopAuthenticator {
	return &NoopAuthenticator{}
}

// Authenticate implements Authenticator.
func (a *NoopAuthenticator) Authenticate(_ context.Context, conn Connection, _ interface{}) (string, error) {
	return conn.Info().RemoteAddr, nil
}

// MTLSAuthenticator validates client certificates for mTLS connections.
type MTLSAuthenticator struct {
	allowedCNs  map[string]struct{}
	allowedOrgs map[string]struct{}
	mu          sync.RWMutex
}

// NewMTLSAuthenticator creates a new mTLS authenticator.
func NewMTLSAuthenticator(config AuthConfig) *MTLSAuthenticator {
	auth := &MTLSAuthenticator{
		allowedCNs:  make(map[string]struct{}),
		allowedOrgs: make(map[string]struct{}),
	}

	for _, cn := range config.AllowedCNs {
		auth.allowedCNs[cn] = struct{}{}
	}

	for _, org := range config.AllowedOrgs {
		auth.allowedOrgs[org] = struct{}{}
	}

	return auth
}

// Authenticate implements Authenticator for mTLS connections.
func (a *MTLSAuthenticator) Authenticate(_ context.Context, conn Connection, credentials interface{}) (string, error) {
	// Try to get the TLS connection state from the credentials
	var state *tls.ConnectionState
	if cs, ok := credentials.(*tls.ConnectionState); ok {
		state = cs
	} else {
		// Try to extract from the connection if it's a TLS connection
		if tlsConn, ok := getUnderlyingTLSConn(conn); ok {
			connState := tlsConn.ConnectionState()
			state = &connState
		}
	}

	if state == nil {
		return "", fmt.Errorf("mTLS authentication requires a TLS connection")
	}

	if len(state.PeerCertificates) == 0 {
		return "", fmt.Errorf("no client certificate provided")
	}

	cert := state.PeerCertificates[0]

	// Validate the certificate
	if err := a.validateCertificate(cert); err != nil {
		return "", err
	}

	return cert.Subject.CommonName, nil
}

// validateCertificate checks if the certificate meets the authentication requirements.
func (a *MTLSAuthenticator) validateCertificate(cert *x509.Certificate) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check Common Name if restrictions are configured
	if len(a.allowedCNs) > 0 {
		if _, ok := a.allowedCNs[cert.Subject.CommonName]; !ok {
			return fmt.Errorf("certificate CN %q not in allowed list", cert.Subject.CommonName)
		}
	}

	// Check Organization if restrictions are configured
	if len(a.allowedOrgs) > 0 {
		found := false
		for _, org := range cert.Subject.Organization {
			if _, ok := a.allowedOrgs[org]; ok {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("certificate organization not in allowed list")
		}
	}

	return nil
}

// AddAllowedCN adds a Common Name to the allowed list.
func (a *MTLSAuthenticator) AddAllowedCN(cn string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowedCNs[cn] = struct{}{}
}

// RemoveAllowedCN removes a Common Name from the allowed list.
func (a *MTLSAuthenticator) RemoveAllowedCN(cn string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.allowedCNs, cn)
}

// AddAllowedOrg adds an Organization to the allowed list.
func (a *MTLSAuthenticator) AddAllowedOrg(org string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowedOrgs[org] = struct{}{}
}

// RemoveAllowedOrg removes an Organization from the allowed list.
func (a *MTLSAuthenticator) RemoveAllowedOrg(org string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.allowedOrgs, org)
}

// TokenAuthenticator validates token-based authentication.
type TokenAuthenticator struct {
	token []byte
	mu    sync.RWMutex
}

// NewTokenAuthenticator creates a new token authenticator.
func NewTokenAuthenticator(token string) *TokenAuthenticator {
	return &TokenAuthenticator{
		token: []byte(token),
	}
}

// Authenticate implements Authenticator for token-based authentication.
// The credentials parameter should be the token string.
func (a *TokenAuthenticator) Authenticate(_ context.Context, conn Connection, credentials interface{}) (string, error) {
	token, ok := credentials.(string)
	if !ok {
		return "", fmt.Errorf("token authentication requires a string token")
	}

	a.mu.RLock()
	expectedToken := a.token
	a.mu.RUnlock()

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(token), expectedToken) != 1 {
		return "", fmt.Errorf("invalid authentication token")
	}

	return conn.Info().RemoteAddr, nil
}

// SetToken updates the expected token.
func (a *TokenAuthenticator) SetToken(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = []byte(token)
}

// ChainAuthenticator chains multiple authenticators together.
// Authentication succeeds if any authenticator succeeds.
type ChainAuthenticator struct {
	authenticators []Authenticator
}

// NewChainAuthenticator creates a new chain authenticator.
func NewChainAuthenticator(authenticators ...Authenticator) *ChainAuthenticator {
	return &ChainAuthenticator{
		authenticators: authenticators,
	}
}

// Authenticate implements Authenticator by trying each authenticator in sequence.
func (a *ChainAuthenticator) Authenticate(ctx context.Context, conn Connection, credentials interface{}) (string, error) {
	var lastErr error
	for _, auth := range a.authenticators {
		identity, err := auth.Authenticate(ctx, conn, credentials)
		if err == nil {
			return identity, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return "", fmt.Errorf("all authenticators failed: %w", lastErr)
	}
	return "", fmt.Errorf("no authenticators configured")
}

// Add adds an authenticator to the chain.
func (a *ChainAuthenticator) Add(auth Authenticator) {
	a.authenticators = append(a.authenticators, auth)
}

// AuthConnectionHandler implements ConnectionHandler with authentication support.
type AuthConnectionHandler struct {
	authenticator Authenticator
	onConnect     func(ctx context.Context, conn Connection, identity string) error
	onDisconnect  func(ctx context.Context, conn Connection, identity string)
}

// NewAuthConnectionHandler creates a new authentication connection handler.
func NewAuthConnectionHandler(authenticator Authenticator) *AuthConnectionHandler {
	return &AuthConnectionHandler{
		authenticator: authenticator,
	}
}

// SetOnConnect sets the callback for connection establishment.
func (h *AuthConnectionHandler) SetOnConnect(fn func(ctx context.Context, conn Connection, identity string) error) {
	h.onConnect = fn
}

// SetOnDisconnect sets the callback for connection termination.
func (h *AuthConnectionHandler) SetOnDisconnect(fn func(ctx context.Context, conn Connection, identity string)) {
	h.onDisconnect = fn
}

// OnConnect implements ConnectionHandler.
func (h *AuthConnectionHandler) OnConnect(ctx context.Context, conn Connection) error {
	identity, err := h.authenticator.Authenticate(ctx, conn, nil)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if h.onConnect != nil {
		return h.onConnect(ctx, conn, identity)
	}
	return nil
}

// OnDisconnect implements ConnectionHandler.
func (h *AuthConnectionHandler) OnDisconnect(ctx context.Context, conn Connection) {
	if h.onDisconnect != nil {
		// Try to get identity from connection info
		identity := conn.Info().AuthIdentity
		if identity == "" {
			identity = conn.Info().RemoteAddr
		}
		h.onDisconnect(ctx, conn, identity)
	}
}

// getUnderlyingTLSConn attempts to extract the TLS connection from a Connection.
func getUnderlyingTLSConn(conn Connection) (*tls.Conn, bool) {
	// Type assertion to check if the underlying connection is a TLS connection
	type tlsConnGetter interface {
		TLSConn() *tls.Conn
	}

	if getter, ok := conn.(tlsConnGetter); ok {
		return getter.TLSConn(), true
	}

	return nil, false
}

// NewAuthenticator creates an authenticator based on the configuration.
func NewAuthenticator(config AuthConfig) (Authenticator, error) {
	switch config.Type {
	case AuthTypeNone, "":
		return NewNoopAuthenticator(), nil
	case AuthTypeMTLS:
		return NewMTLSAuthenticator(config), nil
	case AuthTypeToken:
		if config.Token == "" {
			return nil, fmt.Errorf("token authentication requires a token")
		}
		return NewTokenAuthenticator(config.Token), nil
	default:
		return nil, fmt.Errorf("unknown authentication type: %s", config.Type)
	}
}
