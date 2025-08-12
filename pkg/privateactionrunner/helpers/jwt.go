package helpers

import (
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
)

func Base64ToJWK(privateKey string) (jwk jose.JSONWebKey, err error) {
	decodedKeyBytes, err := base64.RawURLEncoding.DecodeString(privateKey)
	if err != nil {
		return jose.JSONWebKey{}, fmt.Errorf("error decoding private key: %+v", err)
	}
	if err = json.Unmarshal(decodedKeyBytes, &jwk); err != nil {
		return jose.JSONWebKey{}, fmt.Errorf("error converting private key to JWK: %+v", err)
	}
	return jwk, nil
}

func EcdsaToJWK(key any) (*jose.JSONWebKey, error) {
	// Check if the key is a ECDSA key.
	switch key.(type) {
	case *ecdsa.PrivateKey, *ecdsa.PublicKey:
	default:
		return nil, errors.New("unsupported key type")
	}

	// Encode the public key.
	newJwk := jose.JSONWebKey{
		Algorithm: "ES256",
		Key:       key,
		Use:       "sig",
	}

	// Compute the thumbprint of the public key.
	thumbprint, err := newJwk.Thumbprint(crypto.SHA256)
	if err != nil {
		return nil, err
	}

	// Set the key id.
	newJwk.KeyID = base64.RawURLEncoding.EncodeToString(thumbprint)

	return &newJwk, nil
}
func GeneratePARJWT(orgId int64, runnerId string, privateKey *ecdsa.PrivateKey, extraClaims map[string]any) (string, error) {
	claims := jwt.MapClaims{
		"orgId":    orgId,
		"runnerId": runnerId,
		"exp":      time.Now().Add(time.Minute * 1).Unix(),
	}

	for k, v := range extraClaims {
		claims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["alg"] = "ES256"
	token.Header["cty"] = "JWT"

	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("error signing JWT: %w", err)
	}

	return signed, nil
}
