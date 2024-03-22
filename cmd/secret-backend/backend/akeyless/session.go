package akeyless

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type AkeylessSessionBackendConfig struct {
	AkeylessAccessId  string `mapstructure:"akeyless_access_id"`
	AkeylessAccessKey string `mapstructure:"akeyless_access_key"`
}

type authRequest struct {
	AccessId   string `json:"access-id"`
	AccessKey  string `json:"access-key"`
	AccessType string `json:"access-type"`
}

type authResponse struct {
	Token string `json:"token"`
	//	might need to add creds
}

func NewAkeylessConfigFromBackendConfig(akeylessUrl string, sessionConfig AkeylessSessionBackendConfig) (string, error) {
	requestBody, _ := json.Marshal(authRequest{
		AccessId:   sessionConfig.AkeylessAccessId,
		AccessKey:  sessionConfig.AkeylessAccessKey,
		AccessType: "access_key",
	})

	resp, err := http.Post(strings.TrimRight(akeylessUrl, "/")+"/auth", "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", errors.New("failed to authenticate with akeyless")
	}

	defer resp.Body.Close()

	var authResp authResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", err
	}

	return authResp.Token, err
}
