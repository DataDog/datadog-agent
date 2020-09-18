// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/mobile/asset"
)

// called by init in config.go, to ensure any os-specific config is done
// in time
func osinit() {
}

const (
	defaultConfdPath            = ""
	defaultAdditionalChecksPath = ""
	defaultRunPath              = ""
	defaultSyslogURI            = ""
	defaultGuiPort              = 5002
	// defaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	defaultSecurityAgentLogFile = "/var/log/datadog/security-agent.log"
)

func setAssetFs(config Config) {
	afs := NewAssetFs()
	config.SetFs(afs)
}

// AssetFs is a Fs implementation that uses functions provided by the os package.
//
// For details in any method, check the documentation of the os package
// (http://golang.org/pkg/os/).
type AssetFs struct{}

func NewAssetFs() AssetFs {
	log.Printf("Returning new assetfs")
	return AssetFs{}
}

type AssetFile struct {
	asset.File
	FileName string
}

func (AssetFs) Name() string { return "AssetFs" }

func (AssetFs) Create(name string) (afero.File, error) {
	// can't create files in assets, they're read-only
	return nil, errors.New("Invalid Operation: Can't create asset")
}

func (AssetFs) Mkdir(name string, perm os.FileMode) error {
	// can't create files in assets, they're read-only
	return errors.New("Invalid Operation: Can't create directory in asset")
}

func (AssetFs) MkdirAll(path string, perm os.FileMode) error {
	// can't create files in assets, they're read-only
	return errors.New("Invalid Operation: Can't create directory in asset")
}

func (AssetFs) Open(name string) (afero.File, error) {
	log.Printf("assetfs open %s", name)
	f, err := asset.Open(name)
	if err != nil {
		log.Printf("assetfs open %s failed %v", name, err)
		return nil, err
	}
	return AssetFile{File: f, FileName: name}, nil
}

func (AssetFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	log.Printf("assetfs open %s", name)
	f, err := asset.Open(name)
	if err != nil {
		return nil, err
	}
	return AssetFile{File: f, FileName: name}, nil
}

func (AssetFs) Remove(name string) error {
	return errors.New("Invalid Operation: Can't remove file in asset")
}

func (AssetFs) RemoveAll(path string) error {
	return errors.New("Invalid Operation: Can't remove file in asset")
}

func (AssetFs) Rename(oldname, newname string) error {
	return errors.New("Invalid Operation: Can't rename file in asset")
}

func (AssetFs) Stat(name string) (os.FileInfo, error) {
	log.Printf("Returning error for stat %s", name)
	return nil, errors.New("Invalid Operation: Can't stat file in asset")
}

func (AssetFs) Chmod(name string, mode os.FileMode) error {
	return errors.New("Invalid Operation: Can't chmod file in asset")
}

func (AssetFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return errors.New("Invalid Operation: Can't chtimes in asset")
}

func (AssetFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	fi, err := os.Lstat(name)
	return fi, true, err
}

func (f AssetFile) Name() string {
	return f.FileName
}

// ReadAt reads len(b) bytes from the File starting at byte offset off.
// It returns the number of bytes read and the error, if any.
// ReadAt always returns a non-nil error when n < len(b).
// At end of file, that error is io.EOF.
func (f AssetFile) ReadAt(b []byte, off int64) (n int, err error) {
	// for now always just return error
	return 0, errors.New("Invalid Operation: Can't readat asset")
}

func (f AssetFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, errors.New("Invalid Operation: Can't readat asset")
}

func (f AssetFile) Readdirnames(n int) ([]string, error) {
	return nil, errors.New("Invalid Operation: Can't readat asset")
}

func (f AssetFile) Stat() (os.FileInfo, error) {

	return nil, errors.New("Invalid Operation: Can't readat asset")
}

func (f AssetFile) Sync() error {
	return errors.New("Invalid Operation: Can't readat asset")
}
func (f AssetFile) Truncate(size int64) error {
	return errors.New("Invalid Operation: Can't readat asset")
}

// Write writes len(b) bytes to the File.
// It returns the number of bytes written and an error, if any.
// Write returns a non-nil error when n != len(b).
func (f AssetFile) Write(b []byte) (n int, err error) {
	return 0, errors.New("Invalid Operation: Can't readat asset")
}

// WriteString is like Write, but writes the contents of string s rather than
// a slice of bytes.
func (f AssetFile) WriteString(s string) (n int, err error) {
	return f.Write([]byte(s))
}

// WriteAt writes len(b) bytes to the File starting at byte offset off.
// It returns the number of bytes written and an error, if any.
// WriteAt returns a non-nil error when n != len(b).
func (f AssetFile) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, errors.New("Invalid Operation: Can't readat asset")
}

// Seek sets the offset for the next Read or Write on file to offset, interpreted
// according to whence: 0 means relative to the origin of the file, 1 means
// relative to the current offset, and 2 means relative to the end.
// It returns the new offset and an error, if any.
// The behavior of Seek on a file opened with O_APPEND is not specified.
func (f AssetFile) Seek(offset int64, whence int) (ret int64, err error) {
	return 0, errors.New("Invalid Operation: Can't readat asset")
}
