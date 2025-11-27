// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// rotatingNDRecords allows adding timestamped entries to a file
// Adding an entry is efficient and simply appends it to a file, using newlines as a separator.
// We keep this file relatively small in two ways: (1) old entries are pruned from the front of
// the file, and (2) if the file size passes a threshold, it gets rotated to a file name that
// looks like this: "file.000000.txt", "file.000001.txt", etc

type rotatingNDRecords struct {
	filename string
	cfg      config
	// time of the earliest entry in the file
	firstEntry time.Time
	// mtime of the oldest rotated file, nil for uninitialized, zero for "no files"
	oldestFileMtime *time.Time
}

type config struct {
	// how many spaces to use in rotated filenames
	spacer int
	// limit of each file, not to be exceeded
	sizeLimit int64
	// how long to retain old entries
	retention time.Duration
}

// newRotatingNDRecords returns a new rotatingNDRecords
func newRotatingNDRecords(filename string, cfg config) *rotatingNDRecords {
	return &rotatingNDRecords{
		filename: filename,
		cfg:      cfg,
	}
}

// NOTE: if golang ever adds support for general struct field access with generics,
// then this data structure could use generics to allow any element type with a "Time" field
// see https://stackoverflow.com/questions/70358216/how-can-i-access-a-struct-field-with-generics-type-t-has-no-field-or-method
// see https://github.com/golang/go/issues/48522

type ndRecord struct {
	Time time.Time   `json:"time"`
	Data interface{} `json:"data"`
}

// Add adds a new record to the file with the given time and payload
// old entries will be pruned, and the file will be rotated if it gets too large
func (r *rotatingNDRecords) Add(t time.Time, payload interface{}) error {
	r.ensureDefaults()

	// prune old entries
	if !r.firstEntry.IsZero() && t.Sub(r.firstEntry) > r.cfg.retention {
		if err := r.pruneOldEntries(t); err != nil {
			log.Error(err)
		}
	}
	// remove old files that were already rotated
	if !r.oldestFileMtime.IsZero() && t.Sub(*r.oldestFileMtime) > r.cfg.retention {
		if err := r.removeOldFiles(t); err != nil {
			log.Error(err)
		}
	}

	var recordData bytes.Buffer
	err := json.NewEncoder(&recordData).Encode(ndRecord{
		Time: t,
		Data: payload,
	})
	if err != nil {
		return err
	}

	// if new entry will push file over size limit, rotate the file
	if stat, err := os.Stat(r.filename); err == nil {
		if stat.Size()+int64(len(recordData.Bytes())) > r.cfg.sizeLimit {
			r.rotateFile()
		}
	}

	// open the file and append to it
	f, err := os.OpenFile(r.filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(recordData.Bytes())
	return err
}

// RotatedFiles returns list of rotated files
func (r *rotatingNDRecords) RotatedFiles() []string {
	dir := filepath.Dir(r.filename)
	re, err := buildRotationRegex(r.filename, r.cfg.spacer)
	if err != nil {
		log.Error(err)
		return nil
	}

	matches := []string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, ent := range entries {
		if re.MatchString(ent.Name()) {
			matches = append(matches, filepath.Join(dir, ent.Name()))
		}
	}

	return matches
}

func (r *rotatingNDRecords) ensureDefaults() {
	if r.cfg.retention == 0 {
		// default: 90 days
		r.cfg.retention = 90 * 24 * time.Hour
	}
	if r.cfg.spacer == 0 {
		// default: 6 spacer characters
		r.cfg.spacer = 6
	}
	if r.cfg.sizeLimit == 0 {
		// default: 250kb
		r.cfg.sizeLimit = 250000
	}
	if r.firstEntry.IsZero() {
		if f, err := os.OpenFile(r.filename, os.O_RDONLY, 0640); err == nil {
			defer f.Close()
			rd := bufio.NewReader(f)
			if line, err := rd.ReadBytes('\n'); err == nil {
				var rec ndRecord
				if err = json.Unmarshal(line, &rec); err == nil {
					r.firstEntry = rec.Time
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Errorf("opening file: %s", err)
		}
	}
	// mtime of oldest rotated file, if one exists
	if r.oldestFileMtime == nil {
		matches := r.RotatedFiles()
		if len(matches) == 0 {
			// zero value means "no file"
			r.oldestFileMtime = &time.Time{}
		} else if stat, err := os.Stat(matches[0]); err == nil {
			mtime := stat.ModTime()
			r.oldestFileMtime = &mtime
		}
	}
}

func (r *rotatingNDRecords) pruneOldEntries(now time.Time) error {
	var rec ndRecord
	f, err := os.OpenFile(r.filename, os.O_RDONLY, 0640)
	if err != nil {
		return err
	}
	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadBytes('\n')
		if err != nil {
			// reached end of file, invalidate cached time stamp
			r.firstEntry = time.Time{}
			break
		}
		// entries that cannot be parsed, or that have no "time" field, will be pruned
		if err = json.Unmarshal(line, &rec); err == nil {
			if !rec.Time.IsZero() && now.Sub(rec.Time) <= r.cfg.retention {
				r.firstEntry = rec.Time
				break
			}
		}
	}
	tmpForRename, _ := os.CreateTemp("", "replace")
	if !r.firstEntry.IsZero() {
		// if we found an entry to keep, write it first
		json.NewEncoder(tmpForRename).Encode(rec) //nolint:errcheck
	}
	// write the remainder of the file
	if _, err := io.Copy(tmpForRename, rd); err != nil {
		return err
	}
	tmpForRename.Close()
	f.Close()
	return os.Rename(tmpForRename.Name(), r.filename)
}

// remove any old rotated files that are past the retention time
func (r *rotatingNDRecords) removeOldFiles(t time.Time) error {
	for _, filename := range r.RotatedFiles() {
		if stat, err := os.Stat(filename); err == nil {
			if t.Sub(stat.ModTime()) > r.cfg.retention {
				log.Infof("removing old rotated file '%s'", filename)
				os.Remove(filename)
				r.oldestFileMtime = nil
			} else {
				break
			}
		}
	}
	return nil
}

// rotate the current file to the next available name
func (r *rotatingNDRecords) rotateFile() {
	rotateDestFilename, err := nextRotateFilename(r.filename, r.cfg.spacer)
	if err != nil {
		log.Errorf("could not find rotation filename: %s", err)
		return
	}
	if _, err := os.Stat(rotateDestFilename); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(r.filename, rotateDestFilename); err != nil {
			log.Errorf("could not rotate file: %s", err)
			return
		}
		log.Infof("renamed large file '%s' to '%s'", r.filename, rotateDestFilename)
		r.oldestFileMtime = nil
	}
}

// nextRotateFilename calculates filename that should be used for next rotation
func nextRotateFilename(filename string, spacer int) (string, error) {
	dir := filepath.Dir(filename)
	re, err := buildRotationRegex(filename, spacer)
	if err != nil {
		return "", err
	}

	maxSpacerNum := -1
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, ent := range entries {
		match := re.FindStringSubmatch(ent.Name())
		if len(match) > 0 {
			spacerNum, err := strconv.Atoi(match[1])
			if err == nil && spacerNum > maxSpacerNum {
				maxSpacerNum = spacerNum
			}
		}
	}

	return buildRotationName(filename, spacer, maxSpacerNum+1)
}

func buildRotationRegex(filename string, spacer int) (*regexp.Regexp, error) {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	basenoext := strings.TrimSuffix(base, ext)
	// build a regex that matches rotating files
	// for example, a filename like "records.ndjson" with spacer=6
	// would build the regex "records\.(\d{6})\.ndjson"
	pattern := fmt.Sprintf(`%s\.(\d{%d})%s`, regexp.QuoteMeta(basenoext), spacer, regexp.QuoteMeta(ext))
	return regexp.Compile(pattern)
}

func buildRotationName(filename string, spacer, num int) (string, error) {
	if spacer < 1 {
		return "", fmt.Errorf("invalid spacer size: %d", spacer)
	}
	ext := filepath.Ext(filename)
	prefix := strings.TrimSuffix(filename, ext)
	// build a string that matches the names of rotating files
	// for example, a filename like "records.ndjson" with spacer=6 and num=4
	// would build the string "records.000004.ndjson"
	return fmt.Sprintf("%s.%0*d%s", prefix, spacer, num, ext), nil
}
