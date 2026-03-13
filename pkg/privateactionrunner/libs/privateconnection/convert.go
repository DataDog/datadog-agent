// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package privateconnection

import (
	http "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	connlib "github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/connection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func NewPlainTextToken(segments []string, value string) *privateactionspb.ConnectionToken {
	return &privateactionspb.ConnectionToken{
		NameSegments: segments,
		TokenValue: &privateactionspb.ConnectionToken_PlainText_{
			PlainText: &privateactionspb.ConnectionToken_PlainText{Value: value},
		},
	}
}

func NewFileSecretToken(segments []string, path string) *privateactionspb.ConnectionToken {
	return &privateactionspb.ConnectionToken{
		NameSegments: segments,
		TokenValue: &privateactionspb.ConnectionToken_FileSecret_{
			FileSecret: &privateactionspb.ConnectionToken_FileSecret{Path: path},
		},
	}
}

func NewYamlFileToken(segments []string, path string) *privateactionspb.ConnectionToken {
	return &privateactionspb.ConnectionToken{
		NameSegments: segments,
		TokenValue: &privateactionspb.ConnectionToken_YamlFile_{
			YamlFile: &privateactionspb.ConnectionToken_YamlFile{Path: path},
		},
	}
}

func ExtractConnectionDetails(connInfo *privateactionspb.ConnectionInfo) ([]*privateactionspb.ConnectionToken, HttpDetails) {
	group := connlib.GroupTokens(connInfo.Tokens)
	tokens := group[RootTokenGroupName]
	details := HttpDetails{
		Headers:       getHttpHeaders(group[http.HeadersGroupName]),
		Body:          getHttpBody(group[http.BodyGroupName]),
		BaseURL:       getBaseURL(group[http.BaseUrlTokenName]),
		UrlParameters: getHttpUrlParams(group[http.UrlParametersGroupName]),
	}
	return tokens, details
}

func getHttpHeaders(tokens []*privateactionspb.ConnectionToken) []PrivateCredentialsToken {
	headers := make([]PrivateCredentialsToken, 0)
	for _, token := range tokens {
		headers = append(headers, PrivateCredentialsToken{
			Name:  connlib.GetName(token),
			Value: token.GetPlainText().GetValue(),
		})
	}
	return headers
}

func getHttpUrlParams(tokens []*privateactionspb.ConnectionToken) []PrivateCredentialsToken {
	params := make([]PrivateCredentialsToken, 0)
	for _, token := range tokens {
		params = append(params, PrivateCredentialsToken{
			Name:  connlib.GetName(token),
			Value: token.GetPlainText().GetValue(),
		})
	}
	return params
}

func getHttpBody(tokens []*privateactionspb.ConnectionToken) HttpDetailsBody {
	body := HttpDetailsBody{}
	for _, token := range tokens {
		switch connlib.GetName(token) {
		case http.BodyContentTokenName:
			body.Content = token.GetPlainText().GetValue()
		case http.BodyContentTypeTokenName:
			body.ContentType = token.GetPlainText().GetValue()
		}
	}
	return body
}

func getBaseURL(tokens []*privateactionspb.ConnectionToken) string {
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0].GetPlainText().GetValue()
}
