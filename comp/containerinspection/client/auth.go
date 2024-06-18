// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package client

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/comp/containerinspection/api"
	"github.com/golang-jwt/jwt/v5"
)

// Claims represents JWT claims for a request to the client.
type Claims struct {
	InitContainers map[string]api.ContainerSpec

	jwt.RegisteredClaims
}

func (c *Claims) isTimeValid(t time.Time) bool {
	if c.IssuedAt == nil || c.NotBefore == nil || c.ExpiresAt == nil {
		return false
	}

	if t.Before(c.NotBefore.Time) {
		return false
	}

	if t.After(c.ExpiresAt.Time) {
		return false
	}

	return true
}

func NewClaims(r api.MetadataRequest, id string, atTime time.Time, duration time.Duration) *Claims {
	return &Claims{
		InitContainers: r.InitContainers,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        id,
			Issuer:    "cluster-agent/auto_instrumentation",
			Subject:   "trace-agent/container_inspection",
			Audience:  []string{"dd-inspect"},
			IssuedAt:  jwt.NewNumericDate(atTime),
			NotBefore: jwt.NewNumericDate(atTime),
			ExpiresAt: jwt.NewNumericDate(atTime.Add(duration)),
		},
	}
}

var signingMethod = jwt.SigningMethodHS256

func keyFuncForSecret(secret []byte) func(token *jwt.Token) (any, error) {
	return func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != signingMethod.Alg() {
			return nil, errors.New("invalid signing method")
		}
		return secret, nil
	}
}

func ParseClaimsString(in string, keyFunc func(*jwt.Token) (any, error)) (*Claims, error) {
	token, err := jwt.ParseWithClaims(in, &Claims{}, keyFunc)
	if err != nil {
		return nil, err
	}

	c, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid claim type")
	}

	return c, nil
}
