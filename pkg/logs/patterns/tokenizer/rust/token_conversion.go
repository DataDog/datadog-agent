//go:build rust_patterns

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rtokenizer

import (
	"fmt"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	fb "github.com/DataDog/datadog-agent/pkg/logs/patterns/tokenizer/rust/flatbuffers/patterns"
)

const fbVersion = 1

var extractFromOriginalLog atomic.Bool

func init() {
	extractFromOriginalLog.Store(true)
}

// setExtractFromOriginalLog toggles whether token values are extracted from the original
// log text using offsets, or use Rust-normalized values.
// By default (enabled=true), we extract from the original log to avoid normalization
// mismatches and ensure exact original text, especially for complex structured tokens.
// Disabling this (enabled=false) uses Rust-normalized values, which is useful for
// testing and comparing normalized vs original values.
func setExtractFromOriginalLog(enabled bool) {
	extractFromOriginalLog.Store(enabled)
}

func shouldExtractFromOriginalLog() bool {
	return extractFromOriginalLog.Load()
}

// ============================================================================
// Rust Token Types
// ============================================================================

// RustTokenKind represents the token kind from Rust patterns library
// This maps 1:1 with Rust TokenHolder variants
type RustTokenKind string

const (
	// Basic tokens
	RustTokenWord        RustTokenKind = "Word"
	RustTokenWhitespace  RustTokenKind = "Whitespace"
	RustTokenSpecialChar RustTokenKind = "SpecialCharacter"
	RustTokenNumeric     RustTokenKind = "NumericValue"

	// HTTP tokens
	RustTokenHTTPStatus RustTokenKind = "HttpStatusCode"
	RustTokenHTTPMethod RustTokenKind = "HttpMethod"
	RustTokenSeverity   RustTokenKind = "Severity"

	// Date/time tokens
	RustTokenLocalDate      RustTokenKind = "LocalDate"
	RustTokenLocalDateTime  RustTokenKind = "LocalDateTime"
	RustTokenLocalTime      RustTokenKind = "LocalTime"
	RustTokenOffsetDateTime RustTokenKind = "OffsetDateTime"

	// Network tokens
	RustTokenIPv4        RustTokenKind = "IPv4Address"
	RustTokenIPv6        RustTokenKind = "IPv6Address"
	RustTokenEmail       RustTokenKind = "EmailAddress"
	RustTokenRegularName RustTokenKind = "RegularName"

	// Path/URI tokens
	RustTokenAbsolutePath             RustTokenKind = "AbsolutePath"
	RustTokenURI                      RustTokenKind = "URI"
	RustTokenAuthority                RustTokenKind = "Authority"
	RustTokenPathWithQueryAndFragment RustTokenKind = "PathWithQueryAndFragment"

	// Composite/list tokens
	RustTokenList             RustTokenKind = "TokenList"
	RustTokenCollapsibleList  RustTokenKind = "CollapsibleTokenList"
	RustTokenCollapsed        RustTokenKind = "CollapsedToken"
	RustTokenKeyValueSequence RustTokenKind = "KeyValueSequence"

	RustTokenUnknown RustTokenKind = "Unknown"
)

// RustToken represents a single token from the Rust patterns library
// This is an intermediate representation decoded from FlatBuffers
type RustToken struct {
	Kind  RustTokenKind
	Value string

	// Flags (from Rust Word token and consolidation)
	NeverWildcard bool
	HasDigits     bool

	StartOffset int
	EndOffset   int

	// Optional: nested tokens (for TokenList, CollapsibleTokenList, etc.)
	Children []RustToken
}

// ============================================================================
// FlatBuffers Decoding
// ============================================================================

// decodeTokenizeResponse decodes a tokenization response from FlatBuffers
func decodeTokenizeResponse(buf []byte, raw string) (*token.TokenList, error) {
	if len(buf) == 0 {
		return nil, fmt.Errorf("empty FlatBuffers response")
	}

	response := fb.GetRootAsTokenizeResponse(buf, 0)
	if response.Version() != fbVersion {
		return nil, fmt.Errorf("unsupported FlatBuffers version: %d", response.Version())
	}

	// Check for errors
	if response.ErrorCode() != fb.ErrorCodeNone || len(response.Error()) > 0 {
		errMsg := string(response.Error())
		if errMsg == "" {
			errMsg = errorCodeString(response.ErrorCode())
		}
		return nil, fmt.Errorf("tokenization failed: %s", errMsg)
	}

	// Decode tokens
	tokenCount := response.TokensLength()
	tokens := make([]RustToken, 0, tokenCount)
	for i := 0; i < tokenCount; i++ {
		var fbToken fb.Token
		if !response.Tokens(&fbToken, i) {
			continue
		}
		tokens = append(tokens, rustTokenFromFB(&fbToken))
	}

	return rustTokensToTokenListWithRaw(tokens, raw), nil
}

// rustTokenFromFB converts a FlatBuffer token to a Rust token
func rustTokenFromFB(fbToken *fb.Token) RustToken {
	result := RustToken{
		Kind:          rustTokenKindFromFB(fbToken.Kind()),
		Value:         string(fbToken.Value()),
		NeverWildcard: fbToken.NeverWildcard(),
		HasDigits:     fbToken.HasDigits(),
		StartOffset:   int(fbToken.StartOffset()),
		EndOffset:     int(fbToken.EndOffset()),
	}

	childCount := fbToken.ChildrenLength()
	if childCount > 0 {
		result.Children = make([]RustToken, 0, childCount)
		for i := 0; i < childCount; i++ {
			var child fb.Token
			if !fbToken.Children(&child, i) {
				continue
			}
			result.Children = append(result.Children, rustTokenFromFB(&child))
		}
	}

	return result
}

// rustTokenKindFromFB converts a FlatBuffer token kind to a Rust token kind
func rustTokenKindFromFB(kind fb.TokenKind) RustTokenKind {
	switch kind {
	case fb.TokenKindWord:
		return RustTokenWord
	case fb.TokenKindWhitespace:
		return RustTokenWhitespace
	case fb.TokenKindSpecialCharacter:
		return RustTokenSpecialChar
	case fb.TokenKindNumericValue:
		return RustTokenNumeric
	case fb.TokenKindHttpStatusCode:
		return RustTokenHTTPStatus
	case fb.TokenKindHttpMethod:
		return RustTokenHTTPMethod
	case fb.TokenKindSeverity:
		return RustTokenSeverity
	case fb.TokenKindLocalDate:
		return RustTokenLocalDate
	case fb.TokenKindLocalDateTime:
		return RustTokenLocalDateTime
	case fb.TokenKindLocalTime:
		return RustTokenLocalTime
	case fb.TokenKindOffsetDateTime:
		return RustTokenOffsetDateTime
	case fb.TokenKindIPv4Address:
		return RustTokenIPv4
	case fb.TokenKindIPv6Address:
		return RustTokenIPv6
	case fb.TokenKindEmailAddress:
		return RustTokenEmail
	case fb.TokenKindRegularName:
		return RustTokenRegularName
	case fb.TokenKindAbsolutePath:
		return RustTokenAbsolutePath
	case fb.TokenKindURI:
		return RustTokenURI
	case fb.TokenKindAuthority:
		return RustTokenAuthority
	case fb.TokenKindPathWithQueryAndFragment:
		return RustTokenPathWithQueryAndFragment
	case fb.TokenKindTokenList:
		return RustTokenList
	case fb.TokenKindCollapsibleTokenList:
		return RustTokenCollapsibleList
	case fb.TokenKindCollapsedToken:
		return RustTokenCollapsed
	case fb.TokenKindKeyValueSequence:
		return RustTokenKeyValueSequence
	default:
		return RustTokenUnknown
	}
}

// errorCodeString converts a FlatBuffer error code to a string
func errorCodeString(code fb.ErrorCode) string {
	switch code {
	case fb.ErrorCodeNullPointer:
		return "null pointer"
	case fb.ErrorCodeInvalidUtf8:
		return "invalid UTF-8"
	case fb.ErrorCodeSizeLimit:
		return "size limit"
	default:
		return "unknown error"
	}
}

// ============================================================================
// Token Conversion (Rust → Agent)
// ============================================================================

// ToAgentToken converts a RustToken to an Agent Token
func (rt *RustToken) ToAgentToken() token.Token {
	tokenType := rustKindToAgentType(rt.Kind)
	wildcardStatus := token.NotWildcard

	// Determine initial wildcard status based on token type
	switch tokenType {
	case token.TokenWhitespace:
		wildcardStatus = token.NotWildcard
	case token.TokenWord:
		// Words are potential wildcards unless marked NeverWildcard
		if rt.NeverWildcard {
			wildcardStatus = token.NotWildcard
		} else {
			wildcardStatus = token.PotentialWildcard
		}
	default:
		// Structured types (HTTP, IP, dates, etc.) are potential wildcards
		wildcardStatus = token.PotentialWildcard
	}

	return token.Token{
		Type:          tokenType,
		Value:         rt.Value,
		Wildcard:      wildcardStatus,
		NeverWildcard: rt.NeverWildcard,
		HasDigits:     rt.HasDigits,
	}
}

// ToAgentTokenWithRaw converts a RustToken to an Agent Token, preferring raw substrings when offsets are valid.
func (rt *RustToken) ToAgentTokenWithRaw(raw string) token.Token {
	tok := rt.ToAgentToken()
	if shouldExtractFromOriginalLog() && rt.StartOffset >= 0 && rt.EndOffset >= rt.StartOffset && rt.EndOffset <= len(raw) {
		tok.Value = raw[rt.StartOffset:rt.EndOffset]
	}
	return tok
}

// rustTokensToTokenList converts Rust tokens to an Agent TokenList
// This flattens nested structures (TokenList, CollapsibleTokenList, etc.)
func rustTokensToTokenList(rustTokens []RustToken) *token.TokenList {
	tokens := make([]token.Token, 0, len(rustTokens))
	for i := range rustTokens {
		flattenRustToken(&rustTokens[i], &tokens)
	}
	return token.NewTokenListWithTokens(tokens)
}

func rustTokensToTokenListWithRaw(rustTokens []RustToken, raw string) *token.TokenList {
	tokens := make([]token.Token, 0, len(rustTokens))
	for i := range rustTokens {
		flattenRustTokenWithRaw(&rustTokens[i], raw, &tokens)
	}
	return token.NewTokenListWithTokens(tokens)
}

// flattenRustToken recursively flattens a Rust token into the Agent token list
func flattenRustToken(rt *RustToken, tokens *[]token.Token) {
	// Handle composite tokens that contain children
	switch rt.Kind {
	case RustTokenList, RustTokenCollapsibleList:
		// Flatten children recursively
		for i := range rt.Children {
			flattenRustToken(&rt.Children[i], tokens)
		}
	case RustTokenKeyValueSequence:
		// For KV sequences, we can either:
		// 1. Add as a single collapsed token (current approach)
		// 2. Flatten to individual key-value tokens
		// For now, add as a single token with the signature as value
		*tokens = append(*tokens, rt.ToAgentToken())
	default:
		// Regular token - just convert and append
		*tokens = append(*tokens, rt.ToAgentToken())
	}
}

func flattenRustTokenWithRaw(rt *RustToken, raw string, tokens *[]token.Token) {
	switch rt.Kind {
	case RustTokenList, RustTokenCollapsibleList:
		for i := range rt.Children {
			flattenRustTokenWithRaw(&rt.Children[i], raw, tokens)
		}
	case RustTokenKeyValueSequence:
		*tokens = append(*tokens, rt.ToAgentTokenWithRaw(raw))
	default:
		*tokens = append(*tokens, rt.ToAgentTokenWithRaw(raw))
	}
}

// rustKindToAgentType maps Rust token kinds to Agent TokenType
func rustKindToAgentType(kind RustTokenKind) token.TokenType {
	switch kind {
	case RustTokenWord:
		return token.TokenWord
	case RustTokenWhitespace:
		return token.TokenWhitespace
	case RustTokenSpecialChar:
		return token.TokenSpecialChar
	case RustTokenNumeric:
		return token.TokenNumeric
	case RustTokenHTTPStatus:
		return token.TokenHTTPStatus
	case RustTokenHTTPMethod:
		return token.TokenHTTPMethod
	case RustTokenSeverity:
		return token.TokenSeverityLevel
	case RustTokenLocalDate:
		return token.TokenLocalDate
	case RustTokenLocalDateTime:
		return token.TokenLocalDateTime
	case RustTokenLocalTime:
		return token.TokenLocalTime
	case RustTokenOffsetDateTime:
		return token.TokenOffsetDateTime
	case RustTokenIPv4:
		return token.TokenIPv4
	case RustTokenIPv6:
		return token.TokenIPv6
	case RustTokenEmail:
		return token.TokenEmail
	case RustTokenRegularName:
		return token.TokenRegularName
	case RustTokenAbsolutePath:
		return token.TokenAbsolutePath
	case RustTokenURI:
		return token.TokenURI
	case RustTokenAuthority:
		return token.TokenAuthority
	case RustTokenPathWithQueryAndFragment:
		return token.TokenPathWithQueryAndFragment
	case RustTokenKeyValueSequence:
		return token.TokenKeyValueSequence
	case RustTokenCollapsed:
		return token.TokenCollapsedToken
	case RustTokenList, RustTokenCollapsibleList:
		// These should be flattened before reaching here
		return token.TokenUnknown
	default:
		return token.TokenUnknown
	}
}
