// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package session

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// MaxPageTargetBytes bounds the memory one page takes, whatever larger result file
	// size the session allows.
	MaxPageTargetBytes = 16 << 20
	// maxSourcePageBytes is intake's page size limit (intake PAGE-01).
	maxSourcePageBytes = 100 << 20
)

// LogFields are the identifiers every log line about a run carries.
func LogFields(d *Delivery) string {
	return fmt.Sprintf("crawl_id=%s task_id=%s upload_id=%s", d.CrawlID, d.TaskID, d.UploadID)
}

// Uploader turns a run's encoded rows into pages and uploads them in order: the
// descriptor first, then pages that each aim for the result file size target, then the
// finalize. It holds at most one page in memory.
type Uploader struct {
	client        *Client
	delivery      *Delivery
	columns       []Column
	includeSchema bool
	hostname      string
	logPrefix     string
	target        int64
	page          pageBuffer
	pages         int
	rows          int64
	resultBytes   int64
}

// NewUploader checks the columns (ValidateColumns) and that a result file has room for
// rows, and returns the run's uploader. Nothing is sent yet.
func NewUploader(client *Client, d *Delivery, columns []Column, includeSchema bool, hostname, logPrefix string) (*Uploader, *Failure) {
	if f := ValidateColumns(columns, d.Limits, includeSchema); f != nil {
		return nil, f
	}
	host, ok := jsonString(hostname)
	if !ok {
		return nil, Fail(CodeInternalError, "the Agent hostname is not valid UTF-8")
	}
	u := &Uploader{
		client: client, delivery: d, columns: columns, includeSchema: includeSchema, hostname: hostname,
		logPrefix: logPrefix, target: min(d.Limits.MaxFileBytes, MaxPageTargetBytes),
	}
	u.page.init(columns, includeSchema, d, host)
	if int64(u.page.overhead) >= d.Limits.MaxFileBytes {
		return nil, Fail(CodeSchemaTooLarge, "the result file header and schema leave no room for rows within the result file size")
	}
	return u, nil
}

// Register sends the descriptor.
func (u *Uploader) Register(ctx context.Context) error {
	body, err := descriptorJSON(u.columns, u.includeSchema, u.hostname)
	if err != nil {
		return Fail(CodeInternalError, "the descriptor could not be encoded")
	}
	return u.client.RegisterDescriptor(ctx, body)
}

// Add appends one row. It uploads the current page first when the row would take its
// result file past the size target.
func (u *Uploader) Add(ctx context.Context, cells []Cell) error {
	bound := u.page.rowBound(cells)
	if bound > u.delivery.Limits.MaxRowBytes {
		return Fail(CodeRowTooLarge, "a result row is larger than the row size limit of "+strconv.FormatInt(u.delivery.Limits.MaxRowBytes, 10)+" bytes")
	}
	if !u.page.empty() && u.page.fileBound(len(u.page.ends))+bound+1 > u.target {
		if err := u.flush(ctx); err != nil {
			return err
		}
	}
	if err := u.page.add(cells, bound); err != nil {
		return Fail(CodeInternalError, "a page record could not be encoded")
	}
	return nil
}

// Finish uploads the remaining rows, or one empty page for an empty result, then
// finalizes the session with the number of pages intake accepted.
func (u *Uploader) Finish(ctx context.Context) error {
	for !u.page.empty() || u.pages == 0 {
		if err := u.flush(ctx); err != nil {
			return err
		}
	}
	return u.client.Finalize(ctx, u.pages)
}

// Pages is the number of pages intake accepted.
func (u *Uploader) Pages() int { return u.pages }

// Rows is the number of rows in the pages intake accepted.
func (u *Uploader) Rows() int64 { return u.rows }

// flush uploads one page from the start of the buffer. When intake reports the page's
// result file as too large, it sends the first half of its rows under the same page
// index instead; the rows it left out start the next page.
func (u *Uploader) flush(ctx context.Context) error {
	limits := u.delivery.Limits
	n := len(u.page.ends)
	for {
		if int64(u.pages) >= limits.MaxPages {
			return Fail(CodeResultTooLarge, "the result needs more pages than the page count limit of "+strconv.FormatInt(limits.MaxPages, 10))
		}
		bound := u.page.fileBound(n)
		if u.resultBytes+bound > limits.MaxResultBytes {
			return Fail(CodeResultTooLarge, "the result is larger than the total result size limit")
		}
		body := u.page.prefix(n)
		if len(body) > maxSourcePageBytes {
			return Fail(CodeResultTooLarge, "a page is larger than intake's page size limit")
		}
		upload, err := u.client.PutPage(ctx, u.pages, body, int64(n), u.rows)
		if IsIntakeCode(err, "result_file_too_large") {
			if n <= 1 {
				return Fail(CodeRowTooLarge, "a result row is larger than intake's result file limit")
			}
			n /= 2
			continue
		}
		if err != nil {
			return err
		}
		log.Debugf("%s page=%d rows=%d bytes=%d sent_bytes=%d encoding=%s compress_ms=%.1f upload_ms=%.1f gzip_not_smaller=%t",
			u.logPrefix, u.pages, n, len(body), upload.SentBytes, upload.Encoding,
			float64(upload.Compress.Microseconds())/1000, float64(upload.Upload.Microseconds())/1000, upload.GzipNotSmaller)
		u.pages++
		u.rows += int64(n)
		u.resultBytes += bound
		u.page.drop(n)
		return nil
	}
}

// pageBuffer holds the CSV records of one page and the bound on the result file that
// intake derives from them.
type pageBuffer struct {
	nameBytes []int
	overhead  int
	body      bytes.Buffer
	writer    *csv.Writer
	ends      []int   // body offset after each record
	bounds    []int64 // each row's bound
	total     int64   // sum of bounds
	fields    []string
}

func (p *pageBuffer) init(columns []Column, includeSchema bool, d *Delivery, hostToken string) {
	p.fields = make([]string, len(columns))
	p.writer = csv.NewWriter(&p.body)
	for _, c := range columns {
		token, _ := jsonString(c.Name)
		p.nameBytes = append(p.nameBytes, len(token)+1)
	}
	schema := 0
	if includeSchema {
		schema = len(`,"schema":`) + schemaBytes(columns)
	}
	// Header and trailer around the rows, with the widest record offset and org ID.
	p.overhead = len(fmt.Sprintf(`{"contract_version":"1.0.0","crawl_id":%q,"task_id":%q,"record_offset":`, d.CrawlID, d.TaskID)) + 20 +
		schema + len(`,"data":[`) + len(`],"org_id":`) + 20 + len(`,"agent_hostname":`) + len(hostToken) + len(`,"sensitive_data_scanned":false}`)
}

// rowBound bounds one row's JSON object in the result file.
func (p *pageBuffer) rowBound(cells []Cell) int64 {
	bound := int64(2) // braces
	for i, c := range cells {
		bound += int64(p.nameBytes[i] + c.Bound)
		if i > 0 {
			bound++
		}
	}
	return bound
}

// fileBound bounds the result file of the first n rows.
func (p *pageBuffer) fileBound(n int) int64 {
	bound := int64(p.overhead)
	if n == len(p.bounds) {
		bound += p.total
	} else {
		for _, b := range p.bounds[:n] {
			bound += b
		}
	}
	if n > 1 {
		bound += int64(n - 1)
	}
	return bound
}

func (p *pageBuffer) add(cells []Cell, bound int64) error {
	for i, c := range cells {
		p.fields[i] = c.Token
	}
	if err := p.writer.Write(p.fields); err != nil {
		return err
	}
	p.writer.Flush()
	if err := p.writer.Error(); err != nil {
		return err
	}
	p.ends = append(p.ends, p.body.Len())
	p.bounds = append(p.bounds, bound)
	p.total += bound
	return nil
}

func (p *pageBuffer) empty() bool { return len(p.ends) == 0 }

func (p *pageBuffer) prefix(n int) []byte {
	if n == 0 {
		return nil
	}
	return p.body.Bytes()[:p.ends[n-1]]
}

// drop removes the first n rows.
func (p *pageBuffer) drop(n int) {
	if n == 0 {
		return
	}
	if n == len(p.ends) {
		p.body.Reset()
		p.ends, p.bounds, p.total = p.ends[:0], p.bounds[:0], 0
		return
	}
	cut := p.ends[n-1]
	rest := append([]byte(nil), p.body.Bytes()[cut:]...)
	p.body.Reset()
	p.body.Write(rest)
	ends := make([]int, 0, len(p.ends)-n)
	for _, e := range p.ends[n:] {
		ends = append(ends, e-cut)
	}
	p.ends = ends
	p.bounds = append([]int64(nil), p.bounds[n:]...)
	p.total = 0
	for _, b := range p.bounds {
		p.total += b
	}
}
