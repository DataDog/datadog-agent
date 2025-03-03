// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package createandfetchimpl implements the creation and access to the auth_token used to communicate between Agent
// processes.
package createandfetchimpl

import (
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAuthToken),
		fxutil.ProvideOptional[authtoken.Component](),
	)
}

type authToken struct {
	logger          log.Component
	token           string
	ipccert         []byte
	ipckey          []byte
	clientTLSConfig tls.Config
	serverTLSConfig tls.Config
}

var _ authtoken.Component = (*authToken)(nil)

type dependencies struct {
	fx.In

	Conf config.Component
	Log  log.Component
}

func newAuthToken(deps dependencies) (authtoken.Component, error) {
	if err := util.CreateAndSetAuthToken(deps.Conf); err != nil {
		deps.Log.Errorf("could not create auth_token: %s", err)
		return nil, err
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ipccert); !ok {
		return nil, fmt.Errorf("Unable to generate certPool from PERM IPC cert")
	}

	tlsCert, err := tls.X509KeyPair(ipccert, ipckey)
	if err != nil {
		return nil, fmt.Errorf("Unable to generate x509 cert from PERM IPC cert and key")
	}

	return &authToken{
		logger:  deps.Log,
		token:   token,
		ipccert: ipccert,
		ipckey:  ipckey,
		clientTLSConfig: tls.Config{
			RootCAs: certPool,
		},
		serverTLSConfig: tls.Config{
			Certificates: []tls.Certificate{tlsCert},
		},
	}, nil
}

// Get returns the session token
func (at *authToken) Get() string {
	return at.token
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (at *authToken) GetTLSClientConfig() *tls.Config {
	return at.clientTLSConfig.Clone()
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Client
func (at *authToken) GetTLSServerConfig() *tls.Config {
	return at.serverTLSConfig.Clone()
}

func (at *authToken) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
			err = fmt.Errorf("no session token provided")
			http.Error(w, err.Error(), 401)
			at.logger.Warnf("invalid auth token for %s request to %s: %s", r.Method, r.RequestURI, err)
			return
		}

		tok := strings.Split(auth, " ")
		if tok[0] != "Bearer" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
			err = fmt.Errorf("unsupported authorization scheme: %s", tok[0])
			http.Error(w, err.Error(), 401)
			at.logger.Warnf("invalid auth token for %s request to %s: %s", r.Method, r.RequestURI, err)

			return
		}

		// The following comparison must be evaluated in constant time
		if len(tok) < 2 || !constantCompareStrings(tok[1], at.token) {
			err = fmt.Errorf("invalid session token")
			http.Error(w, err.Error(), 403)
			at.logger.Warnf("invalid auth token for %s request to %s: %s", r.Method, r.RequestURI, err)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// constantCompareStrings compares two strings in constant time.
// It uses the subtle.ConstantTimeCompare function from the crypto/subtle package
// to compare the byte slices of the input strings.
// Returns true if the strings are equal, false otherwise.
func constantCompareStrings(src, tgt string) bool {
	return subtle.ConstantTimeCompare([]byte(src), []byte(tgt)) == 1
}
