// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package fileprovider

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
)

type tempFs struct {
	tempDir string
	t       testing.TB
}

func newTempFs(inT testing.TB) tempFs {
	dir := inT.TempDir()
	return tempFs{
		tempDir: dir,
		t:       inT,
	}
}

func (fs *tempFs) path(name string) string {
	return fmt.Sprintf("%s/%s", fs.tempDir, name)
}

func (fs *tempFs) createFileWithTime(name string, time time.Time) {
	_, err := os.Create(fs.path(name))
	if err != nil {
		fs.t.Errorf("Creating a file at path %q failed with err %v", fs.path(name), err)
		return
	}
	err = os.Chtimes(fs.path(name), time, time)
	if err != nil {
		fs.t.Errorf("Changing times of file at path %q failed with err %v", fs.path(name), err)
		return
	}
}

func (fs *tempFs) createFile(name string) {
	_, err := os.Create(fs.path(name))
	assert.Nil(fs.t, err)
}

func (fs *tempFs) mkDir(name string) {
	err := os.Mkdir(fs.path(name), os.ModePerm)
	if err != nil {
		fs.t.Errorf("Failed to create directory at %q", fs.path(name))
	}
}

type ProviderTestSuite struct {
	suite.Suite
	testDir    string
	filesLimit int
}

// newLogSources returns a new log source initialized with the right path.
func (suite *ProviderTestSuite) newLogSources(path string) []*sources.LogSource {
	return []*sources.LogSource{sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})}
}

func (suite *ProviderTestSuite) SetupTest() {
	suite.filesLimit = 3

	// Create temporary directory
	var err error
	suite.testDir = suite.T().TempDir()

	// Create directory tree:
	path := fmt.Sprintf("%s/1", suite.testDir)
	err = os.Mkdir(path, os.ModePerm)
	suite.Nil(err)

	path = fmt.Sprintf("%s/1/1.log", suite.testDir)
	_, err = os.Create(path)
	suite.Nil(err)

	path = fmt.Sprintf("%s/1/2.log", suite.testDir)
	_, err = os.Create(path)
	suite.Nil(err)

	path = fmt.Sprintf("%s/1/3.log", suite.testDir)
	_, err = os.Create(path)
	suite.Nil(err)

	path = fmt.Sprintf("%s/2", suite.testDir)
	err = os.Mkdir(path, os.ModePerm)
	suite.Nil(err)

	path = fmt.Sprintf("%s/2/1.log", suite.testDir)
	_, err = os.Create(path)
	suite.Nil(err)

	path = fmt.Sprintf("%s/2/2.log", suite.testDir)
	_, err = os.Create(path)
	suite.Nil(err)
}

func (suite *ProviderTestSuite) TearDownTest() {
	status.Clear()
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsSpecificFile() {
	path := fmt.Sprintf("%s/1/1.log", suite.testDir)
	fileProvider := NewFileProvider(suite.filesLimit, WildcardUseFileName)
	logSources := suite.newLogSources(path)
	util.CreateSources(logSources)
	files := fileProvider.FilesToTail(true, logSources)

	suite.Equal(1, len(files))
	suite.False(files[0].IsWildcardPath)
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[0].Path)
	suite.Equal(make([]string, 0), logSources[0].Messages.GetMessages())
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsAllFilesFromDirectory() {
	path := fmt.Sprintf("%s/1/*.log", suite.testDir)
	fileProvider := NewFileProvider(suite.filesLimit, WildcardUseFileName)
	logSources := suite.newLogSources(path)
	status.InitStatus(pkgConfig.Datadog, util.CreateSources(logSources))
	files := fileProvider.FilesToTail(true, logSources)

	suite.Equal(3, len(files))
	suite.True(files[0].IsWildcardPath)
	suite.True(files[1].IsWildcardPath)
	suite.True(files[2].IsWildcardPath)
	suite.Equal(fmt.Sprintf("%s/1/3.log", suite.testDir), files[0].Path)
	suite.Equal(fmt.Sprintf("%s/1/2.log", suite.testDir), files[1].Path)
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[2].Path)
	suite.Equal([]string{"3 files tailed out of 3 files matching"}, logSources[0].Messages.GetMessages())
	suite.Equal(
		[]string{
			"The limit on the maximum number of files in use (3) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		},
		status.Get(false).Warnings,
	)
}

func (suite *ProviderTestSuite) TestCollectFilesWildcardFlag() {
	// with wildcard

	path := fmt.Sprintf("%s/1/*.log", suite.testDir)
	fileProvider := NewFileProvider(suite.filesLimit, WildcardUseFileName)
	logSources := suite.newLogSources(path)
	files, err := fileProvider.CollectFiles(logSources[0])
	suite.NoError(err, "searching for files in this directory shouldn't fail")
	for _, file := range files {
		suite.True(file.IsWildcardPath, "this file has been found with a wildcard pattern.")
	}

	// without wildcard

	path = fmt.Sprintf("%s/1/1.log", suite.testDir)
	fileProvider = NewFileProvider(suite.filesLimit, WildcardUseFileName)
	logSources = suite.newLogSources(path)
	files, err = fileProvider.CollectFiles(logSources[0])
	suite.NoError(err, "searching for files in this directory shouldn't fail")
	for _, file := range files {
		suite.False(file.IsWildcardPath, "this file has not been found using a wildcard pattern.")
	}
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsAllFilesFromAnyDirectoryWithRightPermissions() {
	path := fmt.Sprintf("%s/*/*1.log", suite.testDir)
	fileProvider := NewFileProvider(suite.filesLimit, WildcardUseFileName)
	logSources := suite.newLogSources(path)
	util.CreateSources(logSources)
	files := fileProvider.FilesToTail(true, logSources)

	suite.Equal(2, len(files))
	suite.True(files[0].IsWildcardPath)
	suite.True(files[1].IsWildcardPath)
	suite.Equal(fmt.Sprintf("%s/2/1.log", suite.testDir), files[0].Path)
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[1].Path)
	suite.Equal([]string{"2 files tailed out of 2 files matching"}, logSources[0].Messages.GetMessages())
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsSpecificFileWithWildcard() {
	path := fmt.Sprintf("%s/1/?.log", suite.testDir)
	fileProvider := NewFileProvider(suite.filesLimit, WildcardUseFileName)
	logSources := suite.newLogSources(path)
	status.InitStatus(pkgConfig.Datadog, util.CreateSources(logSources))
	files := fileProvider.FilesToTail(true, logSources)

	suite.Equal(3, len(files))
	suite.True(files[0].IsWildcardPath)
	suite.True(files[1].IsWildcardPath)
	suite.True(files[2].IsWildcardPath)
	suite.Equal(fmt.Sprintf("%s/1/3.log", suite.testDir), files[0].Path)
	suite.Equal(fmt.Sprintf("%s/1/2.log", suite.testDir), files[1].Path)
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[2].Path)
	suite.Equal([]string{"3 files tailed out of 3 files matching"}, logSources[0].Messages.GetMessages())
	suite.Equal(
		[]string{
			"The limit on the maximum number of files in use (3) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		},
		status.Get(false).Warnings,
	)
}

func (suite *ProviderTestSuite) TestWildcardPathsAreSorted() {
	filesLimit := 6
	path := fmt.Sprintf("%s/*/*.log", suite.testDir)
	fileProvider := NewFileProvider(filesLimit, WildcardUseFileName)
	logSources := suite.newLogSources(path)
	files := fileProvider.FilesToTail(true, logSources)
	suite.Equal(5, len(files))
	for i := 0; i < len(files); i++ {
		suite.Assert().True(files[i].IsWildcardPath)
	}
	suite.Equal(fmt.Sprintf("%s/1/3.log", suite.testDir), files[0].Path)
	suite.Equal(fmt.Sprintf("%s/2/2.log", suite.testDir), files[1].Path)
	suite.Equal(fmt.Sprintf("%s/1/2.log", suite.testDir), files[2].Path)
	suite.Equal(fmt.Sprintf("%s/2/1.log", suite.testDir), files[3].Path)
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[4].Path)
}

func (suite *ProviderTestSuite) TestNumberOfFilesToTailDoesNotExceedLimit() {
	path := fmt.Sprintf("%s/*/*.log", suite.testDir)
	fileProvider := NewFileProvider(suite.filesLimit, WildcardUseFileName)
	logSources := suite.newLogSources(path)
	status.InitStatus(pkgConfig.Datadog, util.CreateSources(logSources))
	files := fileProvider.FilesToTail(true, logSources)
	suite.Equal(suite.filesLimit, len(files))
	suite.Equal([]string{"3 files tailed out of 5 files matching"}, logSources[0].Messages.GetMessages())
	suite.Equal(
		[]string{
			"The limit on the maximum number of files in use (3) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		},
		status.Get(false).Warnings,
	)
}

func (suite *ProviderTestSuite) TestAllWildcardPathsAreUpdated() {
	filesLimit := 2
	fileProvider := NewFileProvider(filesLimit, WildcardUseFileName)
	logSources := []*sources.LogSource{
		sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/1/*.log", suite.testDir)}),
		sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/2/*.log", suite.testDir)}),
	}
	status.InitStatus(pkgConfig.Datadog, util.CreateSources(logSources))
	files := fileProvider.FilesToTail(true, logSources)
	suite.Equal(2, len(files))
	suite.Equal([]string{"2 files tailed out of 3 files matching"}, logSources[0].Messages.GetMessages())
	suite.Equal(
		[]string{
			"The limit on the maximum number of files in use (2) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		},
		status.Get(false).Warnings,
	)
	suite.Equal([]string{"0 files tailed out of 2 files matching"}, logSources[1].Messages.GetMessages())
	suite.Equal(
		[]string{
			"The limit on the maximum number of files in use (2) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		},
		status.Get(false).Warnings,
	)

	os.Remove(fmt.Sprintf("%s/1/2.log", suite.testDir))
	os.Remove(fmt.Sprintf("%s/1/3.log", suite.testDir))
	os.Remove(fmt.Sprintf("%s/2/2.log", suite.testDir))
	files = fileProvider.FilesToTail(true, logSources)
	suite.Equal(2, len(files))
	suite.Equal([]string{"1 files tailed out of 1 files matching"}, logSources[0].Messages.GetMessages())

	suite.Equal([]string{"1 files tailed out of 1 files matching"}, logSources[1].Messages.GetMessages())
	suite.Equal(
		[]string{
			"The limit on the maximum number of files in use (2) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		},
		status.Get(false).Warnings,
	)

	os.Remove(fmt.Sprintf("%s/2/1.log", suite.testDir))

	files = fileProvider.FilesToTail(true, logSources)
	suite.Equal(1, len(files))
	suite.Equal([]string{"1 files tailed out of 1 files matching"}, logSources[0].Messages.GetMessages())

	suite.Equal([]string{"0 files tailed out of 0 files matching"}, logSources[1].Messages.GetMessages())
}

func (suite *ProviderTestSuite) TestExcludePath() {
	filesLimit := 6
	path := fmt.Sprintf("%s/*/*.log", suite.testDir)
	excludePaths := []string{fmt.Sprintf("%s/2/*.log", suite.testDir)}
	fileProvider := NewFileProvider(filesLimit, WildcardUseFileName)
	logSources := []*sources.LogSource{
		sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path, ExcludePaths: excludePaths}),
	}

	files := fileProvider.FilesToTail(true, logSources)
	suite.Equal(3, len(files))
	for i := 0; i < len(files); i++ {
		suite.Assert().True(files[i].IsWildcardPath)
	}
	suite.Equal(fmt.Sprintf("%s/1/3.log", suite.testDir), files[0].Path)
	suite.Equal(fmt.Sprintf("%s/1/2.log", suite.testDir), files[1].Path)
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[2].Path)
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}

func TestCollectFiles(t *testing.T) {
	t.Run("Invalid Pattern", func(t *testing.T) {
		fileProvider := NewFileProvider(2, WildcardUseFileName)
		source := sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: "//\\///*"})
		files, err := fileProvider.CollectFiles(source)
		assert.Len(t, files, 0)
		assert.Error(t, err)
	})
	t.Run("ReverseLexicographical", func(t *testing.T) {
		fs := newTempFs(t)
		fs.createFile("a")
		fs.createFile("b")
		fs.createFile("c")
		fs.createFile("d")

		fileProvider := NewFileProvider(2, WildcardUseFileName)
		source := sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: fs.path("*")})
		files, err := fileProvider.CollectFiles(source)
		assert.Nil(t, err)
		assert.Len(t, files, 4)
		assert.Equal(t, fs.path("d"), files[0].Path)
		assert.Equal(t, fs.path("c"), files[1].Path)
		assert.Equal(t, fs.path("b"), files[2].Path)
		assert.Equal(t, fs.path("a"), files[3].Path)
	})

	t.Run("Mtime", func(t *testing.T) {
		baseTime := time.Date(2010, time.August, 10, 25, 0, 0, 0, time.UTC)
		fs := newTempFs(t)

		// Given 4 files with descending mtimes
		fs.createFileWithTime("a.log", baseTime.Add(time.Second*4))
		fs.createFileWithTime("q.log", baseTime.Add(time.Second*3))
		fs.createFileWithTime("t.log", baseTime.Add(time.Second*2))
		fs.createFileWithTime("z.log", baseTime.Add(time.Second*1))

		fileProvider := NewFileProvider(2, WildcardUseFileModTime)
		source := sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: fs.path("*")})
		files, err := fileProvider.CollectFiles(source)
		assert.Nil(t, err)
		assert.Len(t, files, 4)
		assert.Equal(t, fs.path("a.log"), files[0].Path)
		assert.Equal(t, fs.path("q.log"), files[1].Path)
		assert.Equal(t, fs.path("t.log"), files[2].Path)
		assert.Equal(t, fs.path("z.log"), files[3].Path)
	})
}

func TestFilesToTail(t *testing.T) {
	t.Run("Reverse Lexicographical - Greedy", func(t *testing.T) {
		t.Run("Two sources", func(t *testing.T) {
			fs := newTempFs(t)

			fs.mkDir("a")
			fs.createFile("a/a")
			fs.createFile("a/b")
			fs.createFile("a/z")
			fs.mkDir("b")
			fs.createFile("b/a")
			fs.createFile("b/b")
			fs.createFile("b/z")

			fileProvider := NewFileProvider(2, WildcardUseFileName)
			sources := []*sources.LogSource{
				sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: fs.path("a/*")}),
				sources.NewLogSource("wildcardTwo", &config.LogsConfig{Type: config.FileType, Path: fs.path("b/*")}),
			}
			files := fileProvider.FilesToTail(true, sources)
			assert.Len(t, files, 2)
			assert.Equal(t, fs.path("a/z"), files[0].Path)
			assert.Equal(t, fs.path("a/b"), files[1].Path)
		})
		t.Run("Many sources", func(t *testing.T) {
			fs := newTempFs(t)

			fs.mkDir("a")
			fs.createFile("a/aaaa")
			fs.createFile("a/abbb")
			fs.createFile("a/accc")
			fs.createFile("a/addd")
			fs.createFile("a/baaa")
			fs.createFile("a/bbbb")
			fs.createFile("a/bccc")
			fs.createFile("a/bddd")
			fs.mkDir("b")
			fs.createFile("b/a")
			fs.createFile("b/b")
			fs.createFile("b/z")

			fileProvider := NewFileProvider(2, WildcardUseFileName)
			sources := []*sources.LogSource{
				sources.NewLogSource("wildcardA", &config.LogsConfig{Type: config.FileType, Path: fs.path("a/a*")}),
				sources.NewLogSource("wildcardB", &config.LogsConfig{Type: config.FileType, Path: fs.path("a/b*")}),
				sources.NewLogSource("wildcardC", &config.LogsConfig{Type: config.FileType, Path: fs.path("a/c*")}),
				sources.NewLogSource("wildcardTwo", &config.LogsConfig{Type: config.FileType, Path: fs.path("b/*")}),
			}
			files := fileProvider.FilesToTail(true, sources)
			assert.Len(t, files, 2)
			assert.Equal(t, fs.path("a/addd"), files[0].Path)
			assert.Equal(t, fs.path("a/accc"), files[1].Path)
		})
	})
	t.Run("Mtime - Greedy", func(t *testing.T) {
		t.Run("Single Source", func(t *testing.T) {
			fs := newTempFs(t)
			baseTime := time.Date(2010, time.August, 10, 25, 0, 0, 0, time.UTC)

			// Given 4 files with descending mtimes
			fs.createFileWithTime("a.log", baseTime.Add(time.Second*4))
			fs.createFileWithTime("q.log", baseTime.Add(time.Second*3))
			fs.createFileWithTime("t.log", baseTime.Add(time.Second*2))
			fs.createFileWithTime("z.log", baseTime.Add(time.Second*1))

			fileProvider := NewFileProvider(2, WildcardUseFileModTime)
			sources := []*sources.LogSource{
				sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: fs.path("*")}),
			}
			files := fileProvider.FilesToTail(true, sources)
			assert.Len(t, files, 2)
			assert.Equal(t, fs.path("a.log"), files[0].Path)
			assert.Equal(t, fs.path("q.log"), files[1].Path)
		})
		t.Run("A few sources", func(t *testing.T) {
			fs := newTempFs(t)
			baseTime := time.Date(2010, time.August, 10, 25, 0, 0, 0, time.UTC)

			// Given 4 files with descending mtimes
			fs.createFileWithTime("abb.log", baseTime.Add(time.Second*4))
			fs.createFileWithTime("aaa.log", baseTime.Add(time.Second*3))
			fs.createFileWithTime("bbb.log", baseTime.Add(time.Second*2))
			fs.createFileWithTime("baa.log", baseTime.Add(time.Second*1))

			fileProvider := NewFileProvider(2, WildcardUseFileModTime)
			sources := []*sources.LogSource{
				sources.NewLogSource("wildcard a", &config.LogsConfig{Type: config.FileType, Path: fs.path("a*")}),
				sources.NewLogSource("wildcard b", &config.LogsConfig{Type: config.FileType, Path: fs.path("b*")}),
			}
			files := fileProvider.FilesToTail(true, sources)
			assert.Len(t, files, 2)
			assert.Equal(t, fs.path("abb.log"), files[0].Path)
			assert.Equal(t, fs.path("aaa.log"), files[1].Path)
		})
	})
	t.Run("Mtime - Global", func(t *testing.T) {
		fs := newTempFs(t)
		baseTime := time.Date(2010, time.August, 10, 25, 0, 0, 0, time.UTC)

		fs.mkDir("a")
		fs.createFileWithTime("a/a", baseTime.Add(time.Second*4))
		fs.createFileWithTime("a/b", baseTime.Add(time.Second*3))
		fs.createFileWithTime("a/c", baseTime.Add(time.Second*8))
		fs.mkDir("b")
		fs.createFileWithTime("b/a", baseTime.Add(time.Second*6))
		fs.createFileWithTime("b/b", baseTime.Add(time.Second*7))
		fs.createFileWithTime("b/c", baseTime.Add(time.Second*8))

		fileProvider := NewFileProvider(2, WildcardUseFileModTime)
		sources := []*sources.LogSource{
			sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: fs.path("a/*")}),
			sources.NewLogSource("wildcardTwo", &config.LogsConfig{Type: config.FileType, Path: fs.path("b/*")}),
		}
		files := fileProvider.FilesToTail(true, sources)
		assert.Len(t, files, 2)
		assert.Equal(t, fs.path("a/c"), files[0].Path)
		assert.Equal(t, fs.path("b/c"), files[1].Path)
	})
}

func BenchmarkApplyOrdering(b *testing.B) {
	b.Run("Mtime", func(b *testing.B) {
		fs := newTempFs(b)
		baseTime := time.Date(2010, time.August, 25, 0, 0, 0, 0, time.UTC)

		fs.createFileWithTime("a.log", baseTime.Add(time.Second*4))
		fs.createFileWithTime("b.log", baseTime.Add(time.Second*2))
		fs.createFileWithTime("c.log", baseTime.Add(time.Second*5))
		fs.createFileWithTime("d.log", baseTime.Add(time.Second*5))

		fileProvider := NewFileProvider(2, WildcardUseFileModTime)
		files := []*tailer.File{
			{Path: fs.path("a.log")},
			{Path: fs.path("b.log")},
			{Path: fs.path("c.log")},
			{Path: fs.path("d.log")},
		}
		for n := 0; n < b.N; n++ {
			fileProvider.applyOrdering(files)
		}
	})

	b.Run("ReverseLexicographical", func(b *testing.B) {
		fs := newTempFs(b)
		baseTime := time.Date(2010, time.August, 25, 0, 0, 0, 0, time.UTC)

		fs.createFileWithTime("a.log", baseTime.Add(time.Second*4))
		fs.createFileWithTime("b.log", baseTime.Add(time.Second*2))
		fs.createFileWithTime("c.log", baseTime.Add(time.Second*5))
		fs.createFileWithTime("d.log", baseTime.Add(time.Second*5))

		fileProvider := NewFileProvider(2, WildcardUseFileName)
		files := []*tailer.File{
			{Path: fs.path("a.log")},
			{Path: fs.path("b.log")},
			{Path: fs.path("c.log")},
			{Path: fs.path("d.log")},
		}
		for n := 0; n < b.N; n++ {
			fileProvider.applyOrdering(files)
		}
	})
}

func TestApplyOrdering(t *testing.T) {
	t.Run("Mtime", func(t *testing.T) {
		fs := newTempFs(t)
		baseTime := time.Date(2010, time.August, 25, 0, 0, 0, 0, time.UTC)

		// Given 4 files with descending mtimes
		fs.createFileWithTime("a.log", baseTime.Add(time.Second*4))
		fs.createFileWithTime("t.log", baseTime.Add(time.Second*2))
		fs.createFileWithTime("q.log", baseTime.Add(time.Second*3))
		fs.createFileWithTime("z.log", baseTime.Add(time.Second*1))

		files := []*tailer.File{
			{Path: fs.path("t.log")},
			{Path: fs.path("a.log")},
			{Path: fs.path("z.log")},
			{Path: fs.path("q.log")},
		}
		// When we apply ordering
		applyModTimeOrdering(files)

		// Then we should see all files in descending mtime order
		assert.Len(t, files, 4)
		assert.Equal(t, fs.path("a.log"), files[0].Path)
		assert.Equal(t, fs.path("q.log"), files[1].Path)
		assert.Equal(t, fs.path("t.log"), files[2].Path)
		assert.Equal(t, fs.path("z.log"), files[3].Path)
	})
	t.Run("Invalid Files with Mtime", func(t *testing.T) {
		fs := newTempFs(t)
		baseTime := time.Date(2010, time.August, 25, 0, 0, 0, 0, time.UTC)

		// Given 3 files with descending mtimes
		fs.createFileWithTime("a.log", baseTime.Add(time.Second*4))
		fs.createFileWithTime("b.log", baseTime.Add(time.Second*3))
		fs.createFileWithTime("c.log", baseTime.Add(time.Second*1))

		files := []*tailer.File{
			{Path: fs.path("z.log")},
			{Path: fs.path("a.log")},
			{Path: fs.path("c.log")},
			{Path: fs.path("b.log")},
		}
		// When we apply ordering
		applyModTimeOrdering(files)
		// Then we should see all files in descending mtime order
		assert.Len(t, files, 4)
		assert.Equal(t, fs.path("a.log"), files[0].Path)
		assert.Equal(t, fs.path("b.log"), files[1].Path)
		assert.Equal(t, fs.path("c.log"), files[2].Path)
		// Even though `z.log` does not exist, it was properly sorted into last place
		assert.Equal(t, fs.path("z.log"), files[3].Path)
	})

	t.Run("Reverse Lexicographical", func(t *testing.T) {
		t.Run("Flat Directory", func(t *testing.T) {
			files := []*tailer.File{
				{Path: "a.log"},
				{Path: "t.log"},
				{Path: "q.log"},
				{Path: "z.log"},
			}
			applyReverseLexicographicalOrdering(files)

			assert.Len(t, files, 4)
			assert.Equal(t, "z.log", files[0].Path)
			assert.Equal(t, "t.log", files[1].Path)
			assert.Equal(t, "q.log", files[2].Path)
			assert.Equal(t, "a.log", files[3].Path)
		})
		t.Run("Multiple Directories dated log file", func(t *testing.T) {
			files := []*tailer.File{
				{Path: "/tmp/1/2018.log"},
				{Path: "/tmp/1/2017.log"},
				{Path: "/tmp/2/2018.log"},
				{Path: "/tmp/1/2016.log"},
			}
			applyReverseLexicographicalOrdering(files)
			assert.Equal(t, "/tmp/2/2018.log", files[0].Path)
			assert.Equal(t, "/tmp/1/2018.log", files[1].Path)
			assert.Equal(t, "/tmp/1/2017.log", files[2].Path)
			assert.Equal(t, "/tmp/1/2016.log", files[3].Path)
		})

		t.Run("Multiple Directories - dated directory", func(t *testing.T) {
			files := []*tailer.File{
				{Path: "/tmp/2020-02-20/error.log"},
				{Path: "/tmp/2020-02-21/error.log"},
				{Path: "/tmp/2020-02-22/error.log"},
			}
			applyReverseLexicographicalOrdering(files)
			assert.Equal(t, "/tmp/2020-02-22/error.log", files[0].Path)
			assert.Equal(t, "/tmp/2020-02-21/error.log", files[1].Path)
			assert.Equal(t, "/tmp/2020-02-20/error.log", files[2].Path)
		})
		t.Run("Multiple Directories - Out of order input", func(t *testing.T) {
			t.Skip() // See FIXME in 'applyOrdering', this test currently fails
			files := []*tailer.File{
				{Path: "/tmp/1/2018.log"},
				{Path: "/tmp/2/2018.log"},
				{Path: "/tmp/3/2016.log"},
				{Path: "/tmp/3/2018.log"},
				{Path: "/tmp/1/2017.log"},
			}
			applyReverseLexicographicalOrdering(files)

			assert.Equal(t, "/tmp/3/2018.log", files[0].Path)
			assert.Equal(t, "/tmp/3/2016.log", files[1].Path)
			assert.Equal(t, "/tmp/2/2018.log", files[2].Path)
			assert.Equal(t, "/tmp/1/2018.log", files[3].Path)
			assert.Equal(t, "/tmp/1/2017.log", files[4].Path)
		})
	})
}

func TestContainerIDInContainerLogFile(t *testing.T) {
	assert := assert.New(t)

	logSource := sources.NewLogSource("mylogsource", nil)
	logSource.SetSourceType(sources.DockerSourceType)
	logSource.Config = &config.LogsConfig{
		Type: config.FileType,
		Path: "/var/log/pods/file-uuid-foo-bar.log",

		Identifier: "abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd",
	}

	// create an empty file that will represent the log file that would have been found in /var/log/containers
	ContainersLogsDir = "/tmp/"
	os.Remove("/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")

	err := os.Symlink("/var/log/pods/file-uuid-foo-bar.log", "/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
	defer func() {
		// cleaning up after the test run
		os.Remove("/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
		os.Remove("/tmp/myapp_my-namespace_myapp-thisisnotacontainerIDevenifthisispointingtothecorrectfile.log")
	}()

	assert.NoError(err, "error while creating the temporary file")

	file := tailer.File{
		Path:           "/var/log/pods/file-uuid-foo-bar.log",
		IsWildcardPath: false,
		Source:         sources.NewReplaceableSource(logSource),
	}

	// we've found a symlink validating that the file we have just scanned is concerning the container we're currently processing for this source
	assert.False(shouldIgnore(true, &file), "the file existing in ContainersLogsDir is pointing to the same container, scanned file should be tailed")

	// now, let's change the container for which we are trying to scan files,
	// because the symlink is pointing from another container, we should ignore
	// that log file
	file.Source.Config().Identifier = "1234123412341234123412341234123412341234123412341234123412341234"
	assert.True(shouldIgnore(true, &file), "the file existing in ContainersLogsDir is not pointing to the same container, scanned file should be ignored")

	// in this scenario, no link is found in /var/log/containers, thus, we should not ignore the file
	os.Remove("/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
	assert.False(shouldIgnore(true, &file), "no files existing in ContainersLogsDir, we should not ignore the file we have just scanned")

	// in this scenario, the file we've found doesn't look like a container ID
	os.Symlink("/var/log/pods/file-uuid-foo-bar.log", "/tmp/myapp_my-namespace_myapp-thisisnotacontainerIDevenifthisispointingtothecorrectfile.log")
	assert.False(shouldIgnore(true, &file), "no container ID found, we don't want to ignore this scanned file")
}

func TestContainerPathsAreCorrectlyIgnored(t *testing.T) {
	fs := newTempFs(t)

	ContainersLogsDir = "/tmp/"
	fs.mkDir("a")
	fs.createFile("a/a.log")
	fs.createFile("a/b.log")
	os.Remove("/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
	os.Remove("/tmp/myapp_my-namespace_myapp-cbadefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")

	// File we want include
	_ = os.Symlink(fs.path("a/a.log"), "/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
	// File we want to ignore (the id is different)
	_ = os.Symlink(fs.path("a/b.log"), "/tmp/myapp_my-namespace_myapp-cbadefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
	defer func() {
		// cleaning up after the test run
		os.Remove("/tmp/myapp_my-namespace_myapp-abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
		os.Remove("/tmp/myapp_my-namespace_myapp-cbadefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd.log")
	}()

	fs.mkDir("b")
	fs.createFile("b/b")

	kubeSource := sources.NewLogSource("mylogsource", nil)
	kubeSource.SetSourceType(sources.KubernetesSourceType)
	kubeSource.Config = &config.LogsConfig{
		Type: config.FileType,
		Path: fs.path("a/*.log"),

		Identifier: "abcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcdabcdefabcdefabcd",
	}

	fileProvider := NewFileProvider(6, WildcardUseFileName)
	sources := []*sources.LogSource{
		kubeSource,
		sources.NewLogSource("wildcardTwo", &config.LogsConfig{Type: config.FileType, Path: fs.path("b/*")}),
	}
	files := fileProvider.FilesToTail(true, sources)
	assert.Len(t, files, 2) // 1 file from k8s source, 1 file from regular file source.
}
