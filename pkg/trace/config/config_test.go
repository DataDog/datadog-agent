package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppSecObfuscationRegexp(t *testing.T) {
	keyRE := New().Obfuscation.AppSec.ParameterKeyRegexp
	for _, v := range []string{
		"password",
		"passwd",
		"pwd",
		"pass",
		"pass_phrase",
		"passPhrase",
		"secret",
		"api_key",
		"apikey",
		"secret_key",
		"secretkey",
		"private_key",
		"privatekey",
		"public_key",
		"publickey",
		"token",
		"api_token",
		"apiToken",
		"consumerId",
		"consumer_ID",
		"consumer_key",
		"consumerKey",
		"consumer_secret",
		"consumerSecret",
		"signature",
		"signed",
		"bearer",
	} {
		t.Run(v, func(t *testing.T) {
			require.True(t, keyRE.MatchString(v))
		})
	}
}
