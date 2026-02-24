// Copyright (c) 2013 - Cloud Instruments Co., Ltd.
//
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
// ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
// (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
// LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
// SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// Package filewriter provides a rolling file writer.
package filewriter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// File and directory permitions.
const (
	defaultFilePermissions      = 0666
	defaultDirectoryPermissions = 0767
)

// Common constants
const (
	rollingLogHistoryDelimiter = "."
)

// RollingNameMode is the type of the rolled file naming mode: prefix, postfix, etc.
type RollingNameMode uint8

// RollingNameMode values
const (
	RollingNameModePostfix = iota
	RollingNameModePrefix
)

// rollerVirtual is an interface that represents all virtual funcs that are
// called in different rolling writer subtypes.
type rollerVirtual interface {
	needsToRoll() bool                                  // Returns true if needs to switch to another file.
	isFileRollNameValid(rname string) bool              // Returns true if logger roll file name (postfix/prefix/etc.) is ok.
	sortFileRollNamesAsc(fs []string) ([]string, error) // Sorts logger roll file names in ascending order of their creation by logger.

	// getNewHistoryRollFileName is called whenever we are about to roll the
	// current log file. It returns the name the current log file should be
	// rolled to.
	getNewHistoryRollFileName(otherHistoryFiles []string) string

	getCurrentFileName() string
}

// rollingFileWriter writes received messages to a file, until time interval passes
// or file exceeds a specified limit. After that the current log file is renamed
// and writer starts to log into a new file. You can set a limit for such renamed
// files count, if you want, and then the rolling writer would delete older ones when
// the files count exceed the specified limit.
type rollingFileWriter struct {
	fileName        string // log file name
	currentDirPath  string
	currentFile     *os.File
	currentName     string
	currentFileSize int64
	fullName        bool
	maxRolls        int
	nameMode        RollingNameMode
	self            rollerVirtual // Used for virtual calls
	rollLock        sync.Mutex
}

func newRollingFileWriter(fpath string, maxr int, namemode RollingNameMode,
	fullName bool) (*rollingFileWriter, error) {
	rw := new(rollingFileWriter)
	rw.currentDirPath, rw.fileName = filepath.Split(fpath)
	if len(rw.currentDirPath) == 0 {
		rw.currentDirPath = "."
	}

	rw.nameMode = namemode
	rw.maxRolls = maxr
	rw.fullName = fullName
	return rw, nil
}

func (rw *rollingFileWriter) hasRollName(file string) bool {
	switch rw.nameMode {
	case RollingNameModePostfix:
		rname := rw.fileName + rollingLogHistoryDelimiter
		return strings.HasPrefix(file, rname)
	case RollingNameModePrefix:
		rname := rollingLogHistoryDelimiter + rw.fileName
		return strings.HasSuffix(file, rname)
	}
	return false
}

func (rw *rollingFileWriter) createFullFileName(originalName, rollname string) string {
	switch rw.nameMode {
	case RollingNameModePostfix:
		return originalName + rollingLogHistoryDelimiter + rollname
	case RollingNameModePrefix:
		return rollname + rollingLogHistoryDelimiter + originalName
	}
	return ""
}

func (rw *rollingFileWriter) getSortedLogHistory() ([]string, error) {
	files, err := getDirFilePaths(rw.currentDirPath, nil, true)
	if err != nil {
		return nil, err
	}
	var validRollNames []string
	for _, file := range files {
		if rw.hasRollName(file) {
			rname := rw.getFileRollName(file)
			if rw.self.isFileRollNameValid(rname) {
				validRollNames = append(validRollNames, rname)
			}
		}
	}
	sortedTails, err := rw.self.sortFileRollNamesAsc(validRollNames)
	if err != nil {
		return nil, err
	}
	validSortedFiles := make([]string, len(sortedTails))
	for i, v := range sortedTails {
		validSortedFiles[i] = rw.createFullFileName(rw.fileName, v)
	}
	return validSortedFiles, nil
}

func (rw *rollingFileWriter) createFileAndFolderIfNeeded() error {
	var err error

	if len(rw.currentDirPath) != 0 {
		err = os.MkdirAll(rw.currentDirPath, defaultDirectoryPermissions)

		if err != nil {
			return err
		}
	}
	rw.currentName = rw.self.getCurrentFileName()
	filePath := filepath.Join(rw.currentDirPath, rw.currentName)

	// This will either open the existing file (without truncating it) or
	// create if necessary. Append mode avoids any race conditions.
	rw.currentFile, err = os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, defaultFilePermissions)
	if err != nil {
		return err
	}

	stat, err := rw.currentFile.Stat()
	if err != nil {
		rw.currentFile.Close()
		rw.currentFile = nil
		return err
	}

	rw.currentFileSize = stat.Size()
	return nil
}

func (rw *rollingFileWriter) deleteOldRolls(history []string) error {
	if rw.maxRolls <= 0 {
		return nil
	}

	rollsToDelete := len(history) - rw.maxRolls
	if rollsToDelete <= 0 {
		return nil
	}

	var err error
	// In all cases (archive files or not) the files should be deleted.
	for i := 0; i < rollsToDelete; i++ {
		// Try best to delete files without breaking the loop.
		if err = tryRemoveFile(filepath.Join(rw.currentDirPath, history[i])); err != nil {
			fmt.Fprintf(os.Stderr, "log: filewriter internal error: %v\n", err)
		}
	}

	return nil
}

func (rw *rollingFileWriter) getFileRollName(fileName string) string {
	switch rw.nameMode {
	case RollingNameModePostfix:
		return fileName[len(rw.fileName+rollingLogHistoryDelimiter):]
	case RollingNameModePrefix:
		return fileName[:len(fileName)-len(rw.fileName+rollingLogHistoryDelimiter)]
	}
	return ""
}

func (rw *rollingFileWriter) roll() error {
	// First, close current file.
	err := rw.currentFile.Close()
	if err != nil {
		return err
	}
	rw.currentFile = nil

	// Current history of all previous log files.
	// For file roller it may be like this:
	//     * ...
	//     * file.log.4
	//     * file.log.5
	//     * file.log.6
	//
	// For date roller it may look like this:
	//     * ...
	//     * file.log.11.Aug.13
	//     * file.log.15.Aug.13
	//     * file.log.16.Aug.13
	// Sorted log history does NOT include current file.
	history, err := rw.getSortedLogHistory()
	if err != nil {
		return err
	}
	// Renames current file to create a new roll history entry
	// For file roller it may be like this:
	//     * ...
	//     * file.log.4
	//     * file.log.5
	//     * file.log.6
	//     n file.log.7  <---- RENAMED (from file.log)
	newHistoryName := rw.createFullFileName(rw.fileName,
		rw.self.getNewHistoryRollFileName(history))

	err = os.Rename(filepath.Join(rw.currentDirPath, rw.currentName), filepath.Join(rw.currentDirPath, newHistoryName))
	if err != nil {
		return err
	}

	// Finally, add the newly added history file to the history archive
	// and, if after that the archive exceeds the allowed max limit, older rolls
	// must the removed/archived.
	history = append(history, newHistoryName)
	if len(history) > rw.maxRolls {
		err = rw.deleteOldRolls(history)
		if err != nil {
			return err
		}
	}

	return nil
}

func (rw *rollingFileWriter) Write(bytes []byte) (n int, err error) {
	rw.rollLock.Lock()
	defer rw.rollLock.Unlock()

	if rw.self.needsToRoll() {
		if err := rw.roll(); err != nil {
			return 0, err
		}
	}

	if rw.currentFile == nil {
		err := rw.createFileAndFolderIfNeeded()
		if err != nil {
			return 0, err
		}
	}

	n, err = rw.currentFile.Write(bytes)
	rw.currentFileSize += int64(n)
	return n, err
}

func (rw *rollingFileWriter) Close() error {
	rw.rollLock.Lock()
	defer rw.rollLock.Unlock()
	if rw.currentFile != nil {
		e := rw.currentFile.Close()
		if e != nil {
			return e
		}
		rw.currentFile = nil
	}
	return nil
}

// --------------------------------------------------
//      Rolling writer by SIZE
// --------------------------------------------------

// RollingFileWriterSize performs roll when file exceeds a specified limit.
type RollingFileWriterSize struct {
	*rollingFileWriter
	maxFileSize int64
}

// NewRollingFileWriterSize creates a new RollingFileWriterSize.
func NewRollingFileWriterSize(fpath string, maxSize int64, maxRolls int, namemode RollingNameMode) (*RollingFileWriterSize, error) {
	rw, err := newRollingFileWriter(fpath, maxRolls, namemode, false)
	if err != nil {
		return nil, err
	}
	rws := &RollingFileWriterSize{rw, maxSize}
	rws.self = rws
	return rws, nil
}

func (rws *RollingFileWriterSize) needsToRoll() bool {
	return rws.currentFileSize >= rws.maxFileSize
}

func (rws *RollingFileWriterSize) isFileRollNameValid(rname string) bool {
	if len(rname) == 0 {
		return false
	}
	_, err := strconv.Atoi(rname)
	return err == nil
}

type rollSizeFileTailsSlice []string

func (p rollSizeFileTailsSlice) Len() int {
	return len(p)
}
func (p rollSizeFileTailsSlice) Less(i, j int) bool {
	v1, _ := strconv.Atoi(p[i])
	v2, _ := strconv.Atoi(p[j])
	return v1 < v2
}
func (p rollSizeFileTailsSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (rws *RollingFileWriterSize) sortFileRollNamesAsc(fs []string) ([]string, error) {
	ss := rollSizeFileTailsSlice(fs)
	sort.Sort(ss)
	return ss, nil
}

func (rws *RollingFileWriterSize) getNewHistoryRollFileName(otherLogFiles []string) string {
	v := 0
	if len(otherLogFiles) != 0 {
		latest := otherLogFiles[len(otherLogFiles)-1]
		v, _ = strconv.Atoi(rws.getFileRollName(latest))
	}
	return strconv.Itoa(v + 1)
}

func (rws *RollingFileWriterSize) getCurrentFileName() string {
	return rws.fileName
}

func (rws *RollingFileWriterSize) String() string {
	return fmt.Sprintf("Rolling file writer (By SIZE): filename: %s, maxFileSize: %v, maxRolls: %v",
		rws.fileName,
		rws.maxFileSize,
		rws.maxRolls)
}

// --------------------------------------------------
//      Helpers
// --------------------------------------------------

// filePathFilter is a filtering creteria function for file path.
// Must return 'false' to set aside the given file.
type filePathFilter func(filePath string) bool

// getDirFilePaths return full paths of the files located in the directory.
// Remark: Ignores files for which fileFilter returns false.
func getDirFilePaths(dirPath string, fpFilter filePathFilter, pathIsName bool) ([]string, error) {
	dfi, err := os.Open(dirPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open directory %s: %w", dirPath, err)
	}
	defer dfi.Close()

	var absDirPath string
	if !filepath.IsAbs(dirPath) {
		absDirPath, err = filepath.Abs(dirPath)
		if err != nil {
			return nil, fmt.Errorf("cannot get absolute path of directory: %s", err.Error())
		}
	} else {
		absDirPath = dirPath
	}

	// TODO: check if dirPath is really directory.
	// Size of read buffer (i.e. chunk of items read at a time).
	rbs := 2 << 5
	filePaths := []string{}

	var fp string
L:
	for {
		// Read directory entities by reasonable chuncks
		// to prevent overflows on big number of files.
		fis, e := dfi.Readdir(rbs)
		switch e {
		// It's OK.
		case nil:
		// Do nothing, just continue cycle.
		case io.EOF:
			break L
		// Indicate that something went wrong.
		default:
			return nil, e
		}
		// THINK: Maybe, use async running.
		for _, fi := range fis {
			// NB: Should work on every Windows and non-Windows OS.
			if isRegular(fi.Mode()) {
				if pathIsName {
					fp = fi.Name()
				} else {
					// Build full path of a file.
					fp = filepath.Join(absDirPath, fi.Name())
				}
				// Check filter condition.
				if fpFilter != nil && !fpFilter(fp) {
					continue
				}
				filePaths = append(filePaths, fp)
			}
		}
	}
	return filePaths, nil
}

func isRegular(m os.FileMode) bool {
	return m&os.ModeType == 0
}

// tryRemoveFile gives a try removing the file
// only ignoring an error when the file does not exist.
func tryRemoveFile(filePath string) (err error) {
	err = os.Remove(filePath)
	if os.IsNotExist(err) {
		err = nil
		return
	}
	return
}
