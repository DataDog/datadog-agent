// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package uploader deals with uploading SymDB data in the JSON format expected
// by the debugger backend.
package uploader

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/google/uuid"
)

// ScopeType represents the type of scope in the SymDB schema
type ScopeType string

const (
	// ScopeTypePackage represents Go packages.
	ScopeTypePackage ScopeType = "package"
	// ScopeTypeStruct represents Go structs (or other types with methods).
	ScopeTypeStruct ScopeType = "struct"
	// ScopeTypeFunction represents Go functions.
	ScopeTypeFunction ScopeType = "function"
	// ScopeTypeMethod represents Go methods (i.e. functions with a receiver).
	ScopeTypeMethod ScopeType = "method"
	// ScopeTypeLocal represents lexical scopes (pairs of {} brackets) inside go
	// functions.
	ScopeTypeLocal ScopeType = "local"
)

// Scope represents a lexical scope in the SymDB schema
type Scope struct {
	ScopeType          ScopeType          `json:"scope_type"`
	Name               string             `json:"name"`
	SourceFile         string             `json:"source_file,omitempty"`
	StartLine          int                `json:"start_line"`
	EndLine            int                `json:"end_line"`
	HasInjectibleLines bool               `json:"has_injectible_lines"`
	InjectibleLines    []LineRange        `json:"injectible_lines,omitempty"`
	LanguageSpecifics  *LanguageSpecifics `json:"language_specifics,omitempty"`
	Symbols            []Symbol           `json:"symbols,omitempty"`
	Scopes             []Scope            `json:"scopes,omitempty"`
}

// SymbolType represents the type of symbol in the SymDB schema
type SymbolType string

const (
	// SymbolTypeField represents a struct field.
	SymbolTypeField SymbolType = "field"
	// SymbolTypeArg represents a function argument.
	SymbolTypeArg SymbolType = "arg"
	// SymbolTypeLocal represents a local variable.
	SymbolTypeLocal SymbolType = "local"
)

// Symbol represents a variable, parameter, or field in the SymDB schema
type Symbol struct {
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	SymbolType SymbolType `json:"symbol_type"`
	Line       *int       `json:"line,omitempty"`
}

// LanguageSpecifics represents Go language-specific data in the SymDB schema.
type LanguageSpecifics struct {
	AvailableLineRanges []LineRange `json:"available_line_ranges,omitempty"`
	GoQualifiedName     string      `json:"go_qualified_name,omitempty"`
	// AgentVersion is the version of the agent that is uploading this scope to
	// SymDB. Only filled in for root scopes.
	AgentVersion string `json:"agent_version,omitempty"`
}

// LineRange represents a range of source lines, inclusive of both ends.
type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// SymDBUploader deals with uploading SymDB data in the JSON format expected by
// the debugger backend.
type SymDBUploader struct {
	url       string
	service   string
	version   string
	runtimeID string
	headers   [][2]string
}

// NewSymDBUploader returns a new SymDBUploader.
func NewSymDBUploader(
	urlStr string,
	service string,
	version string,
	runtimeID string,
	headers ...[2]string,
) *SymDBUploader {
	return &SymDBUploader{
		url:       urlStr,
		service:   service,
		version:   version,
		runtimeID: runtimeID,
		headers:   headers,
	}
}

// UploadInfo contains metadata about a batch of packages to be uploaded to
// SymDB.
type UploadInfo struct {
	// UploadID identifies which logical upload this batch is part of: if packages
	// are split into multiple batches because of size limits, they will all share
	// the same uploadID.
	UploadID uuid.UUID
	// BatchNum is the number of the batch relative to the other batches in this
	// upload. First batch has number 1.
	BatchNum int
	// Final is set if this is the final (or the only) batch of packages for
	// this upload.
	Final bool
}

// UploadBatch uploads a batch the symbols for a batch of packages to SymDB (via
// the trace-agent).
func (s *SymDBUploader) UploadBatch(ctx context.Context, info UploadInfo, packages []Scope) error {
	// Wrap the data in an envelope expected by the debugger backend.
	var buf bytes.Buffer
	buf.WriteString(`{
"service": "` + s.service + `",
"version": "` + s.version + `",
"language": "go",
"upload_id": "` + info.UploadID.String() + `",
"batch_num": ` + strconv.Itoa(info.BatchNum) + `,
"final": ` + strconv.FormatBool(info.Final) + `,
"scopes": `)

	jsonBytes, err := json.Marshal(packages)
	if err != nil {
		return fmt.Errorf("failed to marshal scope: %w", err)
	}
	buf.Write(jsonBytes)
	buf.WriteString("}")

	if err := s.uploadInner(ctx, buf.Bytes()); err != nil {
		return fmt.Errorf("failed to send individual SymDB: %w", err)
	}

	return nil
}

func (s *SymDBUploader) uploadInner(ctx context.Context, symdbData []byte) error {
	// The upload is a multipart containing metadata expected by the event platform
	// and the gzipped SymDB data.

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	compressedData, err := compressSymDBData(symdbData)
	if err != nil {
		return fmt.Errorf("failed to compress SymDB data: %w", err)
	}

	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition", `form-data; name="file"; filename="file.gz"`)
	fileHeader.Set("Content-Type", "application/gzip")

	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return fmt.Errorf("failed to create file part: %w", err)
	}

	if _, err := filePart.Write(compressedData); err != nil {
		return fmt.Errorf("failed to write compressed SymDB data: %w", err)
	}

	eventHeader := make(textproto.MIMEHeader)
	eventHeader.Set("Content-Disposition", `form-data; name="event"; filename="event.json"`)
	eventHeader.Set("Content-Type", "application/json")

	eventPart, err := writer.CreatePart(eventHeader)
	if err != nil {
		return fmt.Errorf("failed to create event part: %w", err)
	}

	meta := []byte(`{
"ddsource": "dd_debugger",
"service": "` + s.service + `",
"runtimeId": "` + s.runtimeID + `",
"debugger": {
	"type": "symdb"
}
}`)
	if _, err := eventPart.Write(meta); err != nil {
		return fmt.Errorf("failed to write event data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, &buf)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for _, keyVal := range s.headers {
		req.Header.Set(keyVal[0], keyVal[1])
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("uploader received error response: status=%d", resp.StatusCode)
	}

	return nil
}

func compressSymDBData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	if _, err := gzWriter.Write(data); err != nil {
		return nil, fmt.Errorf("failed to write to gzip writer: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

func cleanString(s string) string {
	return strings.ReplaceAll(s, " ", "")
}

// ConvertPackageToScope converts a symdb.Package to a Scope.
//
// agentVersion, if not empty, is reported in the uploaded scope as
// language-specific data.
func ConvertPackageToScope(pkg symdb.Package, agentVersion string) Scope {
	scope := Scope{
		ScopeType: ScopeTypePackage,
		Name:      pkg.Name,
		StartLine: 0,
		EndLine:   0,
		Scopes:    make([]Scope, 0, len(pkg.Functions)+len(pkg.Types)),
	}
	if agentVersion != "" {
		scope.LanguageSpecifics = &LanguageSpecifics{
			AgentVersion: agentVersion,
		}
	}

	// Add functions as method scopes.
	for _, fn := range pkg.Functions {
		fnScope := convertFunctionToScope(fn, false)
		scope.Scopes = append(scope.Scopes, fnScope)
	}

	// Add types as struct scopes.
	for _, typ := range pkg.Types {
		typeScope := convertTypeToScope(*typ)
		scope.Scopes = append(scope.Scopes, typeScope)
	}

	return scope
}

// convertFunctionToScope converts a symdb.Function to a Scope
func convertFunctionToScope(fn symdb.Function, isMethod bool) Scope {
	scopeType := ScopeTypeMethod
	if !isMethod {
		scopeType = ScopeTypeFunction
	}

	injectibleLines := make([]LineRange, len(fn.InjectibleLines))
	for i, r := range fn.InjectibleLines {
		injectibleLines[i] = LineRange{
			Start: r[0],
			End:   r[1],
		}
	}
	scope := Scope{
		ScopeType:          scopeType,
		Name:               fn.Name,
		SourceFile:         fn.File,
		StartLine:          fn.StartLine,
		EndLine:            fn.EndLine,
		Symbols:            make([]Symbol, 0, len(fn.Variables)),
		Scopes:             make([]Scope, 0, len(fn.Scopes)),
		HasInjectibleLines: true,
		InjectibleLines:    injectibleLines,
		LanguageSpecifics: &LanguageSpecifics{
			GoQualifiedName: fn.QualifiedName,
		},
	}

	for _, variable := range fn.Variables {
		symbol := convertVariableToSymbol(variable)
		scope.Symbols = append(scope.Symbols, symbol)
	}

	for _, nestedScope := range fn.Scopes {
		localScope := convertScopeToScope(nestedScope, fn.File)
		scope.Scopes = append(scope.Scopes, localScope)
	}

	return scope
}

// convertTypeToScope converts a symdb.Type to a Scope
func convertTypeToScope(typ symdb.Type) Scope {
	scope := Scope{
		ScopeType: ScopeTypeStruct,
		Name:      typ.Name,
		StartLine: 0,
		EndLine:   0,
		Symbols:   make([]Symbol, 0, len(typ.Fields)),
		Scopes:    make([]Scope, 0, len(typ.Methods)),
	}

	for _, field := range typ.Fields {
		symbol := Symbol{
			Name:       field.Name,
			Type:       cleanString(field.Type),
			SymbolType: SymbolTypeField,
		}
		scope.Symbols = append(scope.Symbols, symbol)
	}

	for _, method := range typ.Methods {
		methodScope := convertFunctionToScope(method, true)
		scope.Scopes = append(scope.Scopes, methodScope)
	}

	return scope
}

// convertScopeToScope converts a symdb.Scope to a Scope
func convertScopeToScope(s symdb.Scope, sourceFile string) Scope {
	scope := Scope{
		ScopeType:  ScopeTypeLocal,
		SourceFile: sourceFile,
		StartLine:  s.StartLine,
		EndLine:    s.EndLine,
		Symbols:    make([]Symbol, 0, len(s.Variables)),
		Scopes:     make([]Scope, 0, len(s.Scopes)),
	}

	for _, variable := range s.Variables {
		symbol := convertVariableToSymbol(variable)
		scope.Symbols = append(scope.Symbols, symbol)
	}

	for _, nestedScope := range s.Scopes {
		localScope := convertScopeToScope(nestedScope, sourceFile)
		scope.Scopes = append(scope.Scopes, localScope)
	}

	return scope
}

// convertVariableToSymbol converts a symdb.Variable to a Symbol
func convertVariableToSymbol(variable symdb.Variable) Symbol {
	symbolType := SymbolTypeLocal
	if variable.FunctionArgument {
		symbolType = SymbolTypeArg
	}

	symbol := Symbol{
		Name:       variable.Name,
		Type:       cleanString(variable.TypeName),
		SymbolType: symbolType,
		Line:       &variable.DeclLine,
	}

	// Add language specifics for line ranges if available
	if len(variable.AvailableLineRanges) > 0 {
		ranges := make([]LineRange, len(variable.AvailableLineRanges))
		for i, r := range variable.AvailableLineRanges {
			ranges[i] = LineRange{
				Start: r[0],
				End:   r[1],
			}
		}
		/*symbol.LanguageSpecifics = &LanguageSpecifics{
			AvailableLineRanges: ranges,
		}*/
	}

	return symbol
}
