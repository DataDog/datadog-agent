package akeyless

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/mitchellh/mapstructure"
	"github.com/rapdev-io/datadog-secret-backend/secret"
	"github.com/rs/zerolog/log"
	"net/http"
)

type AkeylessBackendConfig struct {
	AkeylessSession AkeylessSessionBackendConfig `mapstructure:"akeyless_session"`
	BackendType     string                       `mapstructure:"backend_type"`
	AkeylessUrl     string                       `mapstructure:"akeyless_url"`
}

type AkeylessBackend struct {
	BackendId string
	Config    AkeylessBackendConfig
	Token     string
}

type secretRequest struct {
	AccessId      string   `json:"access-id"`
	AccessKey     string   `json:"access-key"`
	AccessType    string   `json:"access-type"`
	Accessibility string   `json:"accessibility"`
	IgnoreCache   string   `json:"ignore-cache"`
	Json          bool     `json:"json"`
	Names         []string `json:"names"`
	Token         string   `json:"token"`
}

type secretResponse map[string]string

func NewAkeylessBackend(backendId string, bc map[string]interface{}) (*AkeylessBackend, error) {
	backendConfig := AkeylessBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Msg("failed to map backend configuration")
		return nil, err
	}

	authToken, err := NewAkeylessConfigFromBackendConfig(backendConfig.AkeylessUrl, backendConfig.AkeylessSession)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Msg("failed to initialize Akeyless session")
		return nil, err
	}

	backend := &AkeylessBackend{
		BackendId: backendId,
		Config:    backendConfig,
		Token:     authToken,
	}
	return backend, nil
}

func (b *AkeylessBackend) GetSecretOutput(secretKey string) secret.SecretOutput {

	payload := secretRequest{
		AccessType:    "access_key",
		Accessibility: "regular",
		IgnoreCache:   "false",
		Json:          true,
		Names:         []string{secretKey},
		Token:         b.Token,
	}

	// Marshal the payload
	requestPayload, err := json.Marshal(payload)
	if err != nil {
		es := err.Error()
		log.Error().
			Str("backend_id", b.BackendId).
			Str("backend_type", b.Config.BackendType).
			Str("secret_key", secretKey).
			Msg("failed to marshal payload")
		return secret.SecretOutput{Value: nil, Error: &es}
	}

	// Prepare the request
	req, err := http.NewRequest("POST", b.Config.AkeylessUrl+"/get-secret-value", bytes.NewBuffer(requestPayload))
	if err != nil {
		es := err.Error()
		log.Error().
			Str("backend_id", b.BackendId).
			Str("backend_type", b.Config.BackendType).
			Str("secret_key", secretKey).
			Msg("failed to create request")
		return secret.SecretOutput{Value: nil, Error: &es}
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		es := err.Error()
		log.Error().
			Str("backend_id", b.BackendId).
			Str("backend_type", b.Config.BackendType).
			Str("secret_key", secretKey).
			Msg("failed to send request")
		return secret.SecretOutput{Value: nil, Error: &es}
	}
	defer resp.Body.Close()

	// Dump the response for debugging
	//respDump, err := httputil.DumpResponse(resp, true)
	//if err != nil {
	//	log.Error().
	//		Str("backend_id", b.BackendId).
	//		Str("backend_type", b.Config.BackendType).
	//		Str("secret_key", secretKey).
	//		Msg("failed to dump response")
	//} else {
	//	log.Info().
	//		Str("backend_id", b.BackendId).
	//		Str("backend_type", b.Config.BackendType).
	//		Str("secret_key", secretKey).
	//		Msgf("Response:\n%s", string(respDump))
	//}

	// Handle the response
	var secretResponse secretResponse
	err = json.NewDecoder(resp.Body).Decode(&secretResponse)
	if err != nil {
		es := err.Error()
		log.Error().
			Str("backend_id", b.BackendId).
			Str("backend_type", b.Config.BackendType).
			Str("secret_key", secretKey).
			Msg("failed to decode response")
		return secret.SecretOutput{Value: nil, Error: &es}
	}

	// Extract the secret value from the response
	secretValue, ok := secretResponse[secretKey]
	if !ok {
		es := errors.New("secret key not found in response").Error()
		log.Error().
			Str("backend_id", b.BackendId).
			Str("backend_type", b.Config.BackendType).
			Str("secret_key", secretKey).
			Msg("failed to retrieve secret from response")
		return secret.SecretOutput{Value: nil, Error: &es}
	}

	return secret.SecretOutput{Value: &secretValue, Error: nil}
}
