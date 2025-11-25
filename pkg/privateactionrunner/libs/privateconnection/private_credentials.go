package privateconnection

import (
	"fmt"
)

type AuthType string

const (
	TokenAuthType AuthType = "Token Auth"
	BasicAuthType AuthType = "Basic Auth"
)

type PrivateCredentials struct {
	Type        AuthType
	Tokens      []PrivateCredentialsToken
	HttpDetails HttpDetails
}

type PrivateCredentialsToken struct {
	Name  string
	Value string
}

type HttpDetails struct {
	BaseURL       string
	Headers       []PrivateCredentialsToken
	UrlParameters []PrivateCredentialsToken
	Body          HttpDetailsBody
}

type HttpDetailsBody struct {
	Content     string
	ContentType string
}

func (p PrivateCredentialsToken) GetNameSegments() []string {
	return []string{p.Name}
}

func (p PrivateCredentials) GetUsernamePasswordBasicAuth() (string, string, error) {
	if p.Type != BasicAuthType {
		return "", "", fmt.Errorf("not a basic auth credential")
	}
	tokens := p.AsTokenMap()
	return tokens[UsernameTokenName], tokens[PasswordTokenName], nil
}

func (p PrivateCredentials) AsTokenMap() map[string]string {
	tokenMap := make(map[string]string)
	for _, token := range p.Tokens {
		tokenMap[token.Name] = token.Value
	}
	return tokenMap
}
