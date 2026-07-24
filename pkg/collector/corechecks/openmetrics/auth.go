// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const defaultAuthTokenPlaceholder = "<TOKEN>"
const defaultAuthTokenExpiration = 300 * time.Second

type authTokenConfig struct {
	reader authTokenReader
	writer *authTokenHeaderWriter
}

type authTokenReader interface {
	read() (string, error)
	reset()
}

type authTokenFileReader struct {
	path    string
	pattern *regexp.Regexp
	token   string
}

type authTokenHeaderWriter struct {
	name        string
	value       string
	placeholder string
}

type authTokenOAuthReader struct {
	url          string
	clientID     string
	clientSecret string
	basicAuth    bool
	options      map[string]interface{}
	token        string
	expiration   time.Time
}

type authTokenDCOSReader struct {
	loginURL       string
	serviceAccount string
	privateKeyPath string
	expiration     time.Duration
	token          string
}

func parseAuthToken(raw map[string]interface{}) (*authTokenConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	readerRaw, ok := raw["reader"]
	if !ok {
		return nil, errors.New("the `auth_token` field must define both `reader` and `writer` settings")
	}
	writerRaw, ok := raw["writer"]
	if !ok {
		return nil, errors.New("the `auth_token` field must define both `reader` and `writer` settings")
	}

	readerConfig, ok := normalizeMap(readerRaw)
	if !ok {
		return nil, errors.New("the `reader` settings of field `auth_token` must be a mapping")
	}
	writerConfig, ok := normalizeMap(writerRaw)
	if !ok {
		return nil, errors.New("the `writer` settings of field `auth_token` must be a mapping")
	}

	readerType, err := requiredString(readerConfig, "type", "the reader `type` of field `auth_token`")
	if err != nil {
		return nil, err
	}
	var reader authTokenReader
	switch readerType {
	case "file":
		reader, err = parseAuthTokenFileReader(readerConfig)
	case "oauth":
		reader, err = parseAuthTokenOAuthReader(readerConfig)
	case "dcos_auth":
		reader, err = parseAuthTokenDCOSReader(readerConfig)
	default:
		return nil, errors.New("unknown `auth_token` reader type, must be one of: dcos_auth, file, oauth")
	}
	if err != nil {
		return nil, err
	}

	writerType, err := requiredString(writerConfig, "type", "the writer `type` of field `auth_token`")
	if err != nil {
		return nil, err
	}
	if writerType != "header" {
		return nil, errors.New("unknown `auth_token` writer type, must be one of: header")
	}

	writer, err := parseAuthTokenHeaderWriter(writerConfig)
	if err != nil {
		return nil, err
	}
	return &authTokenConfig{reader: reader, writer: writer}, nil
}

func parseAuthTokenFileReader(config map[string]interface{}) (*authTokenFileReader, error) {
	path, err := requiredString(config, "path", "the `path` setting of `auth_token` reader")
	if err != nil {
		return nil, err
	}

	var pattern *regexp.Regexp
	if rawPattern, ok := config["pattern"]; ok {
		patternText, ok := rawPattern.(string)
		if !ok {
			return nil, errors.New("the `pattern` setting of `auth_token` reader must be a string")
		}
		compiled, err := regexp.Compile(patternText)
		if err != nil {
			return nil, err
		}
		if compiled.NumSubexp() != 1 {
			return nil, fmt.Errorf("the pattern `%s` setting of `auth_token` reader must define exactly one group", patternText)
		}
		pattern = compiled
	}

	return &authTokenFileReader{path: path, pattern: pattern}, nil
}

func parseAuthTokenOAuthReader(config map[string]interface{}) (*authTokenOAuthReader, error) {
	url, err := requiredString(config, "url", "the `url` setting of `auth_token` reader")
	if err != nil {
		return nil, err
	}
	clientID, err := requiredString(config, "client_id", "the `client_id` setting of `auth_token` reader")
	if err != nil {
		return nil, err
	}
	clientSecret, err := requiredString(config, "client_secret", "the `client_secret` setting of `auth_token` reader")
	if err != nil {
		return nil, err
	}

	basicAuth := false
	if rawBasicAuth, ok := config["basic_auth"]; ok {
		var ok bool
		basicAuth, ok = rawBasicAuth.(bool)
		if !ok {
			return nil, errors.New("the `basic_auth` setting of `auth_token` reader must be a boolean")
		}
	}

	options := map[string]interface{}{}
	if rawOptions, ok := config["options"]; ok {
		if parsedOptions, ok := normalizeMap(rawOptions); ok {
			options = parsedOptions
		}
	}

	return &authTokenOAuthReader{
		url:          url,
		clientID:     clientID,
		clientSecret: clientSecret,
		basicAuth:    basicAuth,
		options:      options,
	}, nil
}

func parseAuthTokenDCOSReader(config map[string]interface{}) (*authTokenDCOSReader, error) {
	loginURL, err := requiredString(config, "login_url", "the `login_url` setting of DC/OS auth token reader")
	if err != nil {
		return nil, err
	}
	serviceAccount, err := requiredString(config, "service_account", "the `service_account` setting of DC/OS auth token reader")
	if err != nil {
		return nil, err
	}
	privateKeyPath, err := requiredString(config, "private_key_path", "the `private_key_path` setting of DC/OS auth token reader")
	if err != nil {
		return nil, err
	}

	expiration := defaultAuthTokenExpiration
	if rawExpiration, ok := config["expiration"]; ok {
		expirationSeconds, ok := rawExpiration.(int)
		if !ok {
			return nil, errors.New("the `expiration` setting of DC/OS auth token reader must be an integer")
		}
		expiration = time.Duration(expirationSeconds) * time.Second
	}

	return &authTokenDCOSReader{
		loginURL:       loginURL,
		serviceAccount: serviceAccount,
		privateKeyPath: privateKeyPath,
		expiration:     expiration,
	}, nil
}

func parseAuthTokenHeaderWriter(config map[string]interface{}) (*authTokenHeaderWriter, error) {
	name, err := requiredString(config, "name", "the `name` setting of `auth_token` writer")
	if err != nil {
		return nil, err
	}

	value := defaultAuthTokenPlaceholder
	if rawValue, ok := config["value"]; ok {
		var ok bool
		value, ok = rawValue.(string)
		if !ok {
			return nil, errors.New("the `value` setting of `auth_token` writer must be a string")
		}
	}

	placeholder := defaultAuthTokenPlaceholder
	if rawPlaceholder, ok := config["placeholder"]; ok {
		var ok bool
		placeholder, ok = rawPlaceholder.(string)
		if !ok {
			return nil, errors.New("the `placeholder` setting of `auth_token` writer must be a string")
		}
		if placeholder == "" {
			return nil, errors.New("the `placeholder` setting of `auth_token` writer cannot be an empty string")
		}
	}
	if !strings.Contains(value, placeholder) {
		return nil, fmt.Errorf("the `value` setting of `auth_token` writer does not contain the placeholder string `%s`", placeholder)
	}

	return &authTokenHeaderWriter{name: name, value: value, placeholder: placeholder}, nil
}

func requiredString(config map[string]interface{}, key string, setting string) (string, error) {
	raw, ok := config[key]
	if !ok {
		return "", fmt.Errorf("%s is required", setting)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", setting)
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", setting)
	}
	return value, nil
}

func (a *authTokenConfig) apply(request *http.Request) error {
	if a == nil {
		return nil
	}
	token, err := a.reader.read()
	if err != nil {
		return err
	}
	request.Header.Set(a.writer.name, strings.Replace(a.writer.value, a.writer.placeholder, token, 1))
	return nil
}

func (a *authTokenConfig) reset() {
	if a != nil {
		a.reader.reset()
	}
}

func (r *authTokenFileReader) read() (string, error) {
	if r.token != "" {
		return r.token, nil
	}
	content, err := os.ReadFile(r.path)
	if err != nil {
		return "", err
	}
	if r.pattern == nil {
		r.token = strings.TrimSpace(string(content))
		return r.token, nil
	}
	match := r.pattern.FindStringSubmatch(string(content))
	if match == nil {
		return "", fmt.Errorf("the pattern `%s` does not match anything in file: %s", r.pattern.String(), r.path)
	}
	r.token = match[1]
	return r.token, nil
}

func (r *authTokenFileReader) reset() {
	r.token = ""
}

func (r *authTokenOAuthReader) read() (string, error) {
	if r.token != "" && time.Now().Before(r.expiration) {
		return r.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if !r.basicAuth {
		form.Set("client_id", r.clientID)
		form.Set("client_secret", r.clientSecret)
	}
	for key, value := range r.options {
		form.Set(key, fmt.Sprint(value))
	}

	request, err := http.NewRequest(http.MethodPost, r.url, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if r.basicAuth {
		request.SetBasicAuth(r.clientID, r.clientSecret)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("OAuth2 client credentials grant failed with status code %d", response.StatusCode)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if oauthError, _ := payload["error"].(string); oauthError != "" {
		return "", fmt.Errorf("OAuth2 client credentials grant error: %s", oauthError)
	}
	token, _ := payload["access_token"].(string)
	if token == "" {
		return "", errors.New("OAuth2 client credentials response did not contain an access_token")
	}

	r.token = token
	r.expiration = time.Now().Add(parseTokenExpiration(payload["expires_in"]))
	return r.token, nil
}

func (r *authTokenOAuthReader) reset() {
	r.token = ""
	r.expiration = time.Time{}
}

func (r *authTokenDCOSReader) read() (string, error) {
	if r.token != "" {
		return r.token, nil
	}

	privateKey, err := loadRSAPrivateKey(r.privateKeyPath)
	if err != nil {
		return "", err
	}
	expiration := time.Now().Add(r.expiration).Unix()
	signedToken, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"uid": r.serviceAccount,
		"exp": expiration,
	}).SignedString(privateKey)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(map[string]interface{}{
		"uid":   r.serviceAccount,
		"token": signedToken,
		"exp":   expiration,
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequest(http.MethodPost, r.loginURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // Mirrors Python DC/OS auth_token behavior.
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("DC/OS auth token request failed with status code %d", response.StatusCode)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return "", err
	}
	token, _ := payload["token"].(string)
	if token == "" {
		return "", errors.New("DC/OS auth token response did not contain a token")
	}
	r.token = token
	return r.token, nil
}

func (r *authTokenDCOSReader) reset() {
	r.token = ""
}

func parseTokenExpiration(raw interface{}) time.Duration {
	switch value := raw.(type) {
	case float64:
		return time.Duration(value) * time.Second
	case string:
		if parsed, err := strconv.Atoi(value); err == nil {
			return time.Duration(parsed) * time.Second
		}
	}
	return defaultAuthTokenExpiration
}

func loadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("unable to decode PEM private key from %s", path)
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key %s is not an RSA private key", path)
	}
	return key, nil
}
