// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package connection

import (
	"strings"
)

type ConnectionToken interface {
	GetNameSegments() []string
}

type GroupedTokens[T ConnectionToken] map[string][]T

func GetName[T ConnectionToken](token T) string {
	return token.GetNameSegments()[len(token.GetNameSegments())-1]
}

func GroupTokens[T ConnectionToken](tokens []T) GroupedTokens[T] {
	response := make(GroupedTokens[T])
	for _, token := range tokens {
		key := getTokenGroupKey(token)

		if _, exists := response[key]; !exists {
			response[key] = []T{}
		}
		response[key] = append(response[key], token)
	}
	return response
}

func GroupTokensByLevel[T ConnectionToken](tokens []T, level int) GroupedTokens[T] {
	response := make(GroupedTokens[T])
	for _, token := range tokens {
		key := getTokenGroupKeyForLevel(token, level)

		if _, exists := response[key]; !exists {
			response[key] = []T{}
		}
		response[key] = append(response[key], token)
	}
	return response
}

func getTokenGroupKey[T ConnectionToken](token T) string {
	segments := token.GetNameSegments()

	if len(segments) > 1 {
		return strings.Join(segments[:len(segments)-1], ".")
	}

	return GetName(token)
}

func getTokenGroupKeyForLevel[T ConnectionToken](token T, level int) string {
	segments := token.GetNameSegments()

	if len(segments) >= level {
		return token.GetNameSegments()[level]
	}

	return GetName(token)
}

func GetSingleToken[T ConnectionToken](tokens []T) T {
	if len(tokens) == 0 {
		t := new(T)
		return *t
	}
	return tokens[0]
}
