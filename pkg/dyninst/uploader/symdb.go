// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

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
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
)

// SymDBRoot represents the root structure for SymDB uploads, following the JSON schema
type SymDBRoot struct {
	Service  string  `json:"service,omitempty"`
	Env      string  `json:"env,omitempty"`
	Version  string  `json:"version,omitempty"`
	Language string  `json:"language"`
	Scopes   []Scope `json:"scopes"`
}

// ScopeType represents the type of scope in the SymDB schema
type ScopeType string

const (
	ScopeTypePackage ScopeType = "module"
	ScopeTypeStruct  ScopeType = "struct"
	ScopeTypeMethod  ScopeType = "method"
	ScopeTypeClosure ScopeType = "closure"
	ScopeTypeLocal   ScopeType = "local"
)

// Scope represents a lexical scope in the SymDB schema
type Scope struct {
	ScopeType         ScopeType          `json:"scope_type"`
	Name              string             `json:"name"`
	SourceFile        string             `json:"source_file,omitempty"`
	StartLine         int                `json:"start_line"`
	EndLine           int                `json:"end_line"`
	LanguageSpecifics *LanguageSpecifics `json:"language_specifics,omitempty"`
	Symbols           []Symbol           `json:"symbols,omitempty"`
	Scopes            []Scope            `json:"scopes,omitempty"`
}

// SymbolType represents the type of symbol in the SymDB schema
type SymbolType string

const (
	SymbolTypeField       SymbolType = "field"
	SymbolTypeStaticField SymbolType = "static_field"
	SymbolTypeArg         SymbolType = "arg"
	SymbolTypeLocal       SymbolType = "local"
)

// Symbol represents a variable, parameter, or field in the SymDB schema
type Symbol struct {
	Name              string             `json:"name"`
	Type              string             `json:"type"`
	SymbolType        SymbolType         `json:"symbol_type"`
	Line              *int               `json:"line,omitempty"`
	LanguageSpecifics *LanguageSpecifics `json:"language_specifics,omitempty"`
}

type LanguageSpecifics struct {
	AvailableLineRanges []LineRange `json:"available_line_ranges,omitempty"`
}

type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type EventMetadata struct {
	DDSource  string `json:"ddsource"`
	Service   string `json:"service"`
	RuntimeID string `json:"runtimeId"`
}

type SymDBUploader struct {
	*batcher
}

func NewSymDBUploader(opts ...Option) *SymDBUploader {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	sender := newSymDBSender(cfg.client, cfg.url.String())
	return &SymDBUploader{
		batcher: newBatcher("symdb", sender, cfg.batcherConfig),
	}
}

// Enqueue adds a SymDB root object to the uploader's queue
func (u *SymDBUploader) Enqueue(eventMetadata *EventMetadata, symdbRoot *SymDBRoot) error {
	msg := map[string]interface{}{
		"metadata": eventMetadata,
		"symdb":    symdbRoot,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	u.enqueue(data)
	return nil
}

func (u *SymDBUploader) Stop() {
	u.stop()
}

type symdbSender struct {
	client *http.Client
	url    string
}

func newSymDBSender(client *http.Client, urlStr string) *symdbSender {
	return &symdbSender{
		client: client,
		url:    urlStr,
	}
}

func (s *symdbSender) send(batch []json.RawMessage) error {
	for _, msgData := range batch {
		var msg map[string]interface{}
		if err := json.Unmarshal(msgData, &msg); err != nil {
			return fmt.Errorf("failed to unmarshal message: %w", err)
		}

		// Marshal both parts separately for multipart upload
		symdbData, err := json.Marshal(msg["symdb"])
		if err != nil {
			return fmt.Errorf("failed to marshal SymDB data: %w", err)
		}

		metadataData, err := json.Marshal(msg["metadata"])
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		if err := s.upload(symdbData, metadataData); err != nil {
			return fmt.Errorf("failed to send individual SymDB: %w", err)
		}
	}

	return nil
}

func (s *symdbSender) upload(symdbData []byte, eventMetadata []byte) error {
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

	if _, err := eventPart.Write(eventMetadata); err != nil {
		return fmt.Errorf("failed to write event data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.url, &buf)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.client.Do(req)
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

// NewSymDBRoot converts symdb.Symbols to the SymDB JSON schema format
func NewSymDBRoot(service, env, version string, symbols symdb.Symbols) *SymDBRoot {
	root := SymDBRoot{
		Service:  service,
		Env:      env,
		Version:  version,
		Language: "python",
		Scopes:   make([]Scope, 0, len(symbols.Packages)),
	}

	for _, pkg := range symbols.Packages {
		packageScope := convertPackageToScope(pkg)
		root.Scopes = append(root.Scopes, packageScope)
	}

	return &root
}

// NewEventMetadata creates a new EventMetadata with the given service
// and runtimeID.
func NewEventMetadata(service, runtimeID string) *EventMetadata {
	return &EventMetadata{
		DDSource:  "dd_debugger",
		Service:   service,
		RuntimeID: runtimeID,
	}
}

// convertPackageToScope converts a symdb.Package to a Scope
func convertPackageToScope(pkg symdb.Package) Scope {
	scope := Scope{
		ScopeType: ScopeTypePackage,
		Name:      pkg.Name,
		StartLine: 0,
		EndLine:   0,
		Scopes:    make([]Scope, 0, len(pkg.Functions)+len(pkg.Types)),
	}

	// Add functions as method scopes
	for _, fn := range pkg.Functions {
		fnScope := convertFunctionToScope(fn, false)
		scope.Scopes = append(scope.Scopes, fnScope)
	}

	// Add types as struct scopes
	for _, typ := range pkg.Types {
		typeScope := convertTypeToScope(typ)
		scope.Scopes = append(scope.Scopes, typeScope)
	}

	return scope
}

// convertFunctionToScope converts a symdb.Function to a Scope
func convertFunctionToScope(fn symdb.Function, isMethod bool) Scope {
	scopeType := ScopeTypeMethod
	if !isMethod {
		scopeType = ScopeTypeMethod // Functions are treated as methods in the schema
	}

	scope := Scope{
		ScopeType:  scopeType,
		Name:       fn.Name,
		SourceFile: fn.File,
		StartLine:  fn.StartLine,
		EndLine:    fn.EndLine,
		Symbols:    make([]Symbol, 0, len(fn.Variables)),
		Scopes:     make([]Scope, 0, len(fn.Scopes)),
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
