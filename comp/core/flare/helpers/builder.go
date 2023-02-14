// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/mholt/archiver/v3"
)

const (
	filePerm = 0644
)

func newBuilder(root string, hostname string) (*builder, error) {
	fb := &builder{
		tmpDir:     root,
		permsInfos: permissionsInfos{},
	}

	fb.flareDir = filepath.Join(fb.tmpDir, hostname)
	if err := os.MkdirAll(fb.flareDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("Could not create root dir '%s' for flare: %v", fb.flareDir, err)
	}

	fb.scrubber = scrubber.New()
	scrubber.AddDefaultReplacers(fb.scrubber)

	// The default scrubber doesn't deal with api keys of other services, for
	// example powerDNS which has an "api_key" field in its YAML configuration.
	// We add a replacer to scrub even those credentials.
	//
	// It is a best effort to match the api key field without matching our
	// own already scrubbed (we don't want to match: "**************************abcde")
	// Basically we allow many special chars while forbidding *.
	//
	// We want the value to be at least 2 characters which will avoid matching the first '"' from the regular
	// replacer for api_key.
	otherAPIKeysRx := regexp.MustCompile(`api_key\s*:\s*[a-zA-Z0-9\\\/\^\]\[\(\){}!|%:;"~><=#@$_\-\+]{2,}`)
	fb.scrubber.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
		Regex: otherAPIKeysRx,
		ReplFunc: func(b []byte) []byte {
			return []byte("api_key: \"********\"")
		},
	})

	logPath, err := fb.PrepareFilePath("flare_creation.log")
	if err != nil {
		return nil, err
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, filePerm)
	if err != nil {
		return nil, fmt.Errorf("Could not create flare_creation.log file: %s", err)
	}
	fb.logFile = f

	return fb, nil
}

// NewFlareBuilder returns a new FlareBuilder ready to be used. You need to call the Save method to archive all the data
// pushed to the flare as well as cleanup the temporary directories created. Not calling 'Save' after NewFlareBuilder
// will leave temporary directory on the file system.
func NewFlareBuilder() (FlareBuilder, error) {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, fmt.Errorf("Could not create temp dir for flare: %s", err)
	}

	// Get hostname, if there's an error in getting the hostname,
	// set the hostname to unknown
	hostname, err := hostname.Get(context.TODO())
	if err != nil {
		hostname = "unknown"
	}
	hostname = validate.CleanHostnameDir(hostname)

	fperm, err := filesystem.NewPermission()
	if err != nil {
		return nil, err
	}
	err = fperm.RemoveAccessToOtherUsers(tmpDir)
	if err != nil {
		return nil, err
	}

	return newBuilder(tmpDir, hostname)
}

// builder implements the FlareBuilder interface
type builder struct {
	// tmpDir is the temp directory to store data before being archived
	tmpDir string
	// flareDir is the top directory to add file to. This is the equivalent to tmpDir/<hostname>
	flareDir string
	// permsInfos stores the original rights for the files/dirs that were copied into the flare
	permsInfos permissionsInfos

	// specialized scrubber for flare content
	scrubber *scrubber.Scrubber

	logFile *os.File
}

func getArchiveName() string {
	t := time.Now().UTC()
	timeString := strings.ReplaceAll(t.Format(time.RFC3339), ":", "-")

	logLevel, err := log.GetLogLevel()
	logLevelString := ""
	if err == nil {
		logLevelString = fmt.Sprintf("-%s", logLevel.String())
	}

	return fmt.Sprintf("datadog-agent-%s%s.zip", timeString, logLevelString)
}

func (fb *builder) Save() (string, error) {
	defer fb.clean()

	_ = fb.AddFileFromFunc("permissions.log", fb.permsInfos.commit)
	_ = fb.logFile.Close()

	archiveName := getArchiveName()
	archiveTmpPath := filepath.Join(fb.tmpDir, archiveName)
	archiveFinalPath := filepath.Join(os.TempDir(), archiveName)

	// We first create the archive in our fb.tmpDir directory which is only readable by the current user (and
	// SYSTEM/ADMIN on Windows). Then we retrict the archive permissions before moving it to the system temporary
	// directory. This prevents other users from being able to read local flares.

	// File format is determined based on archivePath extension, so zip
	err := archiver.Archive([]string{fb.flareDir}, archiveTmpPath)
	if err != nil {
		return "", err
	}

	fperm, err := filesystem.NewPermission()
	if err != nil {
		return "", err
	}
	err = fperm.RemoveAccessToOtherUsers(archiveTmpPath)
	if err != nil {
		return "", err
	}

	return archiveFinalPath, os.Rename(archiveTmpPath, archiveFinalPath)
}

func (fb *builder) clean() {
	os.RemoveAll(fb.tmpDir)
}

func (fb *builder) logError(format string, params ...interface{}) error {
	err := log.Errorf(format, params...)
	_, _ = fb.logFile.WriteString(err.Error() + "\n")
	return err
}

func (fb *builder) AddFileFromFunc(destFile string, cb func() ([]byte, error)) error {
	content, err := cb()
	if err != nil {
		return fb.logError("error collecting data from callback for '%s': %s", destFile, err)
	}

	return fb.AddFile(destFile, content)
}

func (fb *builder) addFile(shouldScrub bool, destFile string, content []byte) error {
	if shouldScrub {
		var err error
		content, err = fb.scrubber.ScrubBytes(content)
		if err != nil {
			return fb.logError("error scrubbing content for '%s': %s", destFile, err)
		}
	}

	f, err := fb.PrepareFilePath(destFile)
	if err != nil {
		return err
	}

	if err := os.WriteFile(f, content, filePerm); err != nil {
		return fb.logError("error writing data to '%s': %s", destFile, err)
	}
	return nil
}

func (fb *builder) AddFile(destFile string, content []byte) error {
	return fb.addFile(true, destFile, content)
}

func (fb *builder) AddFileWithoutScrubbing(destFile string, content []byte) error {
	return fb.addFile(false, destFile, content)
}

func (fb *builder) copyFileTo(shouldScrub bool, srcFile string, destFile string) error {
	fb.permsInfos.add(srcFile)

	path, err := fb.PrepareFilePath(destFile)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(srcFile)
	if err != nil {
		return fb.logError("error reading file '%s' to be copy to '%s': %s", srcFile, destFile, err)
	}

	if shouldScrub {
		var err error
		content, err = fb.scrubber.ScrubBytes(content)
		if err != nil {
			return fb.logError("error scrubbing content for file '%s': %s", destFile, err)
		}
	}

	err = os.WriteFile(path, content, filePerm)
	if err != nil {
		return fb.logError("error writing file '%s': %s", destFile, err)
	}

	return nil
}

func (fb *builder) CopyFileTo(srcFile string, destFile string) error {
	return fb.copyFileTo(true, srcFile, destFile)
}

func (fb *builder) CopyFile(srcFile string) error {
	return fb.copyFileTo(true, srcFile, filepath.Base(srcFile))
}

func (fb *builder) copyDirTo(shouldScrub bool, srcDir string, destDir string, shouldInclude func(string) bool) error {
	srcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return fb.logError("error getting absolute path for '%s': %s", srcDir, err)
	}
	fb.permsInfos.add(srcDir)

	err = filepath.Walk(srcDir, func(src string, f os.FileInfo, err error) error {
		if f == nil {
			return nil
		}
		if f.IsDir() {
			return nil
		}

		if !shouldInclude(src) {
			return nil
		}

		targetFile := filepath.Join(destDir, strings.Replace(src, srcDir, "", 1))
		_ = fb.copyFileTo(shouldScrub, src, targetFile)
		return nil
	})
	if err != nil {
		return fb.logError("error walking dir '%s': %s", srcDir, err)
	}
	return nil
}

func (fb *builder) CopyDirToWithoutScrubbing(srcDir string, destDir string, shouldInclude func(string) bool) error {
	return fb.copyDirTo(false, srcDir, destDir, shouldInclude)
}

func (fb *builder) CopyDirTo(srcDir string, destDir string, shouldInclude func(string) bool) error {
	return fb.copyDirTo(true, srcDir, destDir, shouldInclude)
}

func (fb *builder) PrepareFilePath(path string) (string, error) {
	p := filepath.Join(fb.flareDir, path)

	err := os.MkdirAll(filepath.Dir(p), os.ModePerm)
	if err != nil {
		return "", fb.logError("error creating directory for file '%s': %s", path, err)
	}
	return p, nil
}

func (fb *builder) RegisterFilePerm(path string) {
	fb.permsInfos.add(path)
}

func (fb *builder) RegisterDirPerm(path string) {
	_ = filepath.Walk(path, func(src string, f os.FileInfo, err error) error {
		if f != nil {
			fb.RegisterFilePerm(src)
		}
		return nil
	})
}
