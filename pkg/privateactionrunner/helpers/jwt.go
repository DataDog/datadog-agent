package helpers

import (
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

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
