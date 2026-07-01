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
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"slices"
	"strconv"
	"strings"

	jsonv2 "github.com/go-json-experiment/json"
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
)

// DefaultFlushThresholdBytes is the default compressed-size threshold at which
// a BatchEncoder will flush an HTTP chunk.
const DefaultFlushThresholdBytes = 2 * 1024 * 1024

// ErrUpload is returned (wrapped) by BatchEncoder.Flush when the HTTP upload
// step fails. Callers can use errors.Is(err, ErrUpload) to distinguish
// upload-side failures (typically retryable) from local encoder errors.
var ErrUpload = errors.New("symdb upload failed")

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
	Name              string             `json:"name"`
	Type              string             `json:"type"`
	SymbolType        SymbolType         `json:"symbol_type"`
	Line              *int               `json:"line,omitempty"`
	LanguageSpecifics *LanguageSpecifics `json:"language_specifics,omitempty"`
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

// uploader holds the destination and identity metadata used by a
// BatchEncoder when shipping a batch to the SymDB intake.
type uploader struct {
	url       string
	service   string
	version   string
	runtimeID string
	headers   [][2]string
}

// bodySink is the storage backend a BatchEncoder writes the full HTTP
// request body into — multipart prologue, gzipped JSON payload, event
// metadata, and closing boundary. Two implementations are provided:
// memSink (a bytes.Buffer, the default) and diskSink (a DiskCache-backed
// file). The interface lets callers spend bounded RAM on the upload
// body by spilling it to disk when a DiskCache is available.
type bodySink interface {
	io.Writer
	// Size reports the number of bytes written since the last Reset.
	Size() int
	// Reader returns an io.Reader that reads back the bytes written so far,
	// from the start. It must be called only after writes for the current
	// batch are complete and before Reset.
	Reader() (io.Reader, error)
	// Reset discards any data buffered for the current batch and prepares
	// the sink to accept the next batch.
	Reset() error
	// Close releases any resources held by the sink. After Close the sink
	// must not be used again.
	Close() error
}

// memSink is a bodySink backed by an in-memory bytes.Buffer.
type memSink struct {
	buf bytes.Buffer
}

func (s *memSink) Write(p []byte) (int, error) { return s.buf.Write(p) }
func (s *memSink) Size() int                   { return s.buf.Len() }
func (s *memSink) Reader() (io.Reader, error)  { return bytes.NewReader(s.buf.Bytes()), nil }
func (s *memSink) Reset() error                { s.buf.Reset(); return nil }
func (s *memSink) Close() error                { s.buf = bytes.Buffer{}; return nil }

// diskSinkMaxBytes caps how large a single batch may grow on disk. With the
// default flush threshold of 2 MiB this is generous, but it bounds the
// reservation if a caller forgets to Flush.
const diskSinkMaxBytes = 64 * 1024 * 1024

// diskSink is a bodySink backed by a DiskCache-managed file. The file
// is unlinked at creation time so it disappears with the process even on a
// crash. Each batch is written, read back for upload, and then truncated.
type diskSink struct {
	df *object.DiskFile
}

func newDiskSink(dc *object.DiskCache) (*diskSink, error) {
	df, err := dc.NewFile("symdb-upload", diskSinkMaxBytes, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create disk-backed upload buffer: %w", err)
	}
	return &diskSink{df: df}, nil
}

func (s *diskSink) Write(p []byte) (int, error) { return s.df.Write(p) }
func (s *diskSink) Size() int                   { return int(s.df.Used()) }

func (s *diskSink) Reader() (io.Reader, error) {
	return io.NewSectionReader(s.df, 0, int64(s.df.Used())), nil
}

func (s *diskSink) Reset() error { return s.df.Truncate() }
func (s *diskSink) Close() error { return s.df.Close() }

// BatchEncoder streams Scope objects into a gzip-compressed SymDB JSON
// envelope wrapped in a multipart upload body, and ships finalised batches
// to the SymDB intake when the caller invokes Flush. A single BatchEncoder
// corresponds to one logical upload (one UploadID) and may emit multiple
// batches. The full request body is written into the underlying sink, so
// the in-flight upload need not be held in memory beyond what the sink
// itself buffers.
type BatchEncoder struct {
	up             uploader
	uploadID       uuid.UUID
	batchNum       int
	sink           bodySink
	mw             *multipart.Writer
	filePartCounts *countingWriter
	gz             *gzip.Writer
	scopeCount     int
	prefixWritten  bool
}

// countingWriter wraps an io.Writer and counts the bytes that flow through it.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// NewBatchEncoder creates a BatchEncoder for a single logical upload to the
// SymDB intake at url.
//
// diskCache is allowed to be nil. When nil, the gzip-compressed batch
// buffer is held in memory. When non-nil, the BatchEncoder spills the
// buffer to a DiskCache-managed file (unlinked at creation) instead;
// failure to allocate the disk file is reported as an error rather than
// silently falling back.
//
// headers are attached to every upload request.
func NewBatchEncoder(
	url string,
	service string,
	version string,
	runtimeID string,
	uploadID uuid.UUID,
	diskCache *object.DiskCache,
	headers [][2]string,
) (*BatchEncoder, error) {
	var sink bodySink
	if diskCache != nil {
		ds, err := newDiskSink(diskCache)
		if err != nil {
			return nil, err
		}
		sink = ds
	} else {
		sink = &memSink{}
	}
	return &BatchEncoder{
		up: uploader{
			url:       url,
			service:   service,
			version:   version,
			runtimeID: runtimeID,
			headers:   headers,
		},
		uploadID: uploadID,
		sink:     sink,
	}, nil
}

// AddScope writes a single Scope into the current batch's gzip stream. On the
// first call of a new batch, it also writes the JSON envelope prefix.
func (b *BatchEncoder) AddScope(scope Scope) error {
	return b.addEncoded(scope)
}

// AddPackage streams a symdb.Package directly into the current batch's gzip
// stream as a "package" scope, without first allocating an intermediate
// uploader.Scope tree.
func (b *BatchEncoder) AddPackage(pkg symdb.Package, agentVersion string) error {
	return b.addEncoded(NewPackageScope(pkg, agentVersion))
}

func (b *BatchEncoder) addEncoded(v any) error {
	if !b.prefixWritten {
		if err := b.startBatch(); err != nil {
			return err
		}
	}
	if b.scopeCount > 0 {
		if _, err := b.gz.Write([]byte{','}); err != nil {
			return fmt.Errorf("failed to write scope separator: %w", err)
		}
	}
	if err := jsonv2.MarshalWrite(b.gz, v); err != nil {
		return fmt.Errorf("failed to encode scope: %w", err)
	}
	if err := b.gz.Flush(); err != nil {
		return fmt.Errorf("failed to flush gzip writer: %w", err)
	}
	b.scopeCount++
	return nil
}

// startBatch writes the multipart prologue (leading boundary + file part
// headers) to the sink, opens a gzip writer over the file part, and writes
// the JSON envelope prefix into it. Subsequent gzip writes flow through
// the multipart file part; Flush closes the gzip writer (which finalises
// the file part body) and then writes the event part + closing boundary.
func (b *BatchEncoder) startBatch() error {
	b.batchNum++
	b.mw = multipart.NewWriter(b.sink)
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition", `form-data; name="file"; filename="file.gz"`)
	fileHeader.Set("Content-Type", "application/gzip")
	filePart, err := b.mw.CreatePart(fileHeader)
	if err != nil {
		return fmt.Errorf("failed to write multipart file header: %w", err)
	}
	b.filePartCounts = &countingWriter{w: filePart}
	if b.gz == nil {
		b.gz = gzip.NewWriter(b.filePartCounts)
	} else {
		b.gz.Reset(b.filePartCounts)
	}
	// JSON-encode service and version, in case they contain funky
	// characters.
	serviceJSON, err := jsonv2.Marshal(b.up.service)
	if err != nil {
		return fmt.Errorf("failed to marshal service: %w", err)
	}
	versionJSON, err := jsonv2.Marshal(b.up.version)
	if err != nil {
		return fmt.Errorf("failed to marshal version: %w", err)
	}
	prefix := `{"service":` + string(serviceJSON) +
		`,"version":` + string(versionJSON) +
		`,"language":"go","upload_id":"` + b.uploadID.String() +
		`","batch_num":` + strconv.Itoa(b.batchNum) +
		`,"scopes":[`
	if _, err := b.gz.Write([]byte(prefix)); err != nil {
		return fmt.Errorf("failed to write envelope prefix: %w", err)
	}
	b.prefixWritten = true
	return nil
}

// Size reports the number of bytes currently buffered for the in-progress
// batch. This counts the multipart prologue + file part headers + the
// gzipped JSON written so far. The gzip writer buffers internally
// (~32 KiB deflate window), so Size is a lower bound on the eventual
// flushed payload size.
func (b *BatchEncoder) Size() int {
	return b.sink.Size()
}

// BatchCount returns the number of batches that have been started so far
// (each AddScope after a Flush starts a new batch). Useful for logging.
func (b *BatchEncoder) BatchCount() int {
	return b.batchNum
}

// Close releases any resources held by the encoder (in particular, a
// DiskCache-managed file when WithDiskCache was used). It is safe to call
// Close after Flush even if Flush failed; the encoder is unusable
// afterwards.
func (b *BatchEncoder) Close() error {
	if b.sink == nil {
		return nil
	}
	err := b.sink.Close()
	b.sink = nil
	return err
}

// Flush finalises the current batch (if any scopes have been added) and
// uploads it. If no scopes have been added since the last flush, this is a
// no-op even when final is true. After Flush returns, the encoder is ready
// to accept scopes for the next batch.
//
// Errors from the HTTP upload step are wrapped with ErrUpload so callers can
// distinguish them with errors.Is.
func (b *BatchEncoder) Flush(ctx context.Context, final bool) error {
	if !b.prefixWritten {
		return nil
	}
	suffix := `],"final":` + strconv.FormatBool(final) + `}`
	if _, err := b.gz.Write([]byte(suffix)); err != nil {
		return fmt.Errorf("failed to write envelope suffix: %w", err)
	}
	if err := b.gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}
	// Emit the inter-part boundary + event part header, then the event JSON
	// body, then the closing boundary. The multipart.Writer handles the
	// boundary framing.
	eventHeader := make(textproto.MIMEHeader)
	eventHeader.Set("Content-Disposition", `form-data; name="event"; filename="event.json"`)
	eventHeader.Set("Content-Type", "application/json")
	eventPart, err := b.mw.CreatePart(eventHeader)
	if err != nil {
		return fmt.Errorf("failed to write multipart event header: %w", err)
	}

	// Some of the fields in here are duplicated inside the envelope in the
	// attachment file; that's on purpose: having them in this message allows
	// the debugger backend to have enough information for its bookkeeping
	// without downloading the attachment. Having the info in the attachment is
	// useful in order to make attachments self-contained.
	event := struct {
		DDSource       string    `json:"ddsource"`
		Service        string    `json:"service"`
		Version        string    `json:"version"`
		Language       string    `json:"language"`
		RuntimeID      string    `json:"runtimeId"`
		Type           string    `json:"type"`
		UploadID       uuid.UUID `json:"uploadId"`
		BatchNum       int       `json:"batchNum"`
		Final          bool      `json:"final"`
		AttachmentSize int64     `json:"attachmentSize"`
	}{
		DDSource:       "dd_debugger",
		Service:        b.up.service,
		Version:        b.up.version,
		Language:       "go",
		RuntimeID:      b.up.runtimeID,
		Type:           "symdb",
		UploadID:       b.uploadID,
		BatchNum:       b.batchNum,
		Final:          final,
		AttachmentSize: b.filePartCounts.n,
	}
	meta, err := jsonv2.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event meta: %w", err)
	}
	if _, err := eventPart.Write(meta); err != nil {
		return fmt.Errorf("failed to write event data: %w", err)
	}
	if err := b.mw.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}
	r, err := b.sink.Reader()
	if err != nil {
		return fmt.Errorf("failed to read back upload body: %w", err)
	}
	if err := b.up.uploadInner(ctx, r, b.mw.FormDataContentType()); err != nil {
		return fmt.Errorf("%w: %w", ErrUpload, err)
	}
	if err := b.sink.Reset(); err != nil {
		return fmt.Errorf("failed to reset upload buffer: %w", err)
	}
	b.mw = nil
	b.scopeCount = 0
	b.prefixWritten = false
	return nil
}

// uploadInner POSTs body to s.url with the given multipart Content-Type.
// body is the entire request body (file + event parts + boundaries) as
// already-framed bytes coming from the BatchEncoder's sink.
func (s *uploader) uploadInner(ctx context.Context, body io.Reader, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, body)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
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
	for _, s := range pkg.Types {
		scope.Scopes = append(scope.Scopes, convertTypeToScope(*s))
	}
	// Sort the types for stable output.
	slices.SortFunc(scope.Scopes, func(a, b Scope) int {
		return strings.Compare(a.Name, b.Name)
	})

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
			Start: int(r[0]),
			End:   int(r[1]),
		}
	}
	scope := Scope{
		ScopeType:          scopeType,
		Name:               fn.Name,
		SourceFile:         fn.File,
		StartLine:          int(fn.StartLine),
		EndLine:            int(fn.EndLine),
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
			Type:       field.Type,
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
		StartLine:  int(s.StartLine),
		EndLine:    int(s.EndLine),
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

	declLine := int(variable.DeclLine)
	symbol := Symbol{
		Name:       variable.Name,
		Type:       variable.TypeName,
		SymbolType: symbolType,
		Line:       &declLine,
	}

	if len(variable.AvailableLineRanges) > 0 {
		ranges := make([]LineRange, len(variable.AvailableLineRanges))
		for i, r := range variable.AvailableLineRanges {
			ranges[i] = LineRange{
				Start: int(r[0]),
				End:   int(r[1]),
			}
		}
		symbol.LanguageSpecifics = &LanguageSpecifics{
			AvailableLineRanges: ranges,
		}
	}

	return symbol
}
