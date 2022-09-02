// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package file_provider

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

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
	fileProvider := NewFileProvider(suite.filesLimit, SortReverseLexicographical, GreedySelection)
	logSources := suite.newLogSources(path)
	util.CreateSources(logSources)
	files := fileProvider.FilesToTail(logSources)

	suite.Equal(1, len(files))
	suite.False(files[0].IsWildcardPath)
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[0].Path)
	suite.Equal(make([]string, 0), logSources[0].Messages.GetMessages())
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsAllFilesFromDirectory() {
	path := fmt.Sprintf("%s/1/*.log", suite.testDir)
	fileProvider := NewFileProvider(suite.filesLimit, SortReverseLexicographical, GreedySelection)
	logSources := suite.newLogSources(path)
	status.InitStatus(util.CreateSources(logSources))
	files := fileProvider.FilesToTail(logSources)

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
		status.Get().Warnings,
	)
}

func (suite *ProviderTestSuite) TestCollectFilesWildcardFlag() {
	// with wildcard

	path := fmt.Sprintf("%s/1/*.log", suite.testDir)
	fileProvider := NewFileProvider(suite.filesLimit, SortReverseLexicographical, GreedySelection)
	logSources := suite.newLogSources(path)
	files, err := fileProvider.CollectFiles(logSources[0])
	suite.NoError(err, "searching for files in this directory shouldn't fail")
	for _, file := range files {
		suite.True(file.IsWildcardPath, "this file has been found with a wildcard pattern.")
	}

	// without wildcard

	path = fmt.Sprintf("%s/1/1.log", suite.testDir)
	fileProvider = NewFileProvider(suite.filesLimit, SortReverseLexicographical, GreedySelection)
	logSources = suite.newLogSources(path)
	files, err = fileProvider.CollectFiles(logSources[0])
	suite.NoError(err, "searching for files in this directory shouldn't fail")
	for _, file := range files {
		suite.False(file.IsWildcardPath, "this file has not been found using a wildcard pattern.")
	}
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsAllFilesFromAnyDirectoryWithRightPermissions() {
	path := fmt.Sprintf("%s/*/*1.log", suite.testDir)
	fileProvider := NewFileProvider(suite.filesLimit, SortReverseLexicographical, GreedySelection)
	logSources := suite.newLogSources(path)
	util.CreateSources(logSources)
	files := fileProvider.FilesToTail(logSources)

	suite.Equal(2, len(files))
	suite.True(files[0].IsWildcardPath)
	suite.True(files[1].IsWildcardPath)
	suite.Equal(fmt.Sprintf("%s/2/1.log", suite.testDir), files[0].Path)
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[1].Path)
	suite.Equal([]string{"2 files tailed out of 2 files matching"}, logSources[0].Messages.GetMessages())
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsSpecificFileWithWildcard() {
	path := fmt.Sprintf("%s/1/?.log", suite.testDir)
	fileProvider := NewFileProvider(suite.filesLimit, SortReverseLexicographical, GreedySelection)
	logSources := suite.newLogSources(path)
	status.InitStatus(util.CreateSources(logSources))
	files := fileProvider.FilesToTail(logSources)

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
		status.Get().Warnings,
	)
}

func (suite *ProviderTestSuite) TestWildcardPathsAreSorted() {
	filesLimit := 6
	path := fmt.Sprintf("%s/*/*.log", suite.testDir)
	fileProvider := NewFileProvider(filesLimit, SortReverseLexicographical, GreedySelection)
	logSources := suite.newLogSources(path)
	files := fileProvider.FilesToTail(logSources)
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
	fileProvider := NewFileProvider(suite.filesLimit, SortReverseLexicographical, GreedySelection)
	logSources := suite.newLogSources(path)
	status.InitStatus(util.CreateSources(logSources))
	files := fileProvider.FilesToTail(logSources)
	suite.Equal(suite.filesLimit, len(files))
	suite.Equal([]string{"3 files tailed out of 5 files matching"}, logSources[0].Messages.GetMessages())
	suite.Equal(
		[]string{
			"The limit on the maximum number of files in use (3) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		},
		status.Get().Warnings,
	)
}

func (suite *ProviderTestSuite) TestAllWildcardPathsAreUpdated() {
	filesLimit := 2
	fileProvider := NewFileProvider(filesLimit, SortReverseLexicographical, GreedySelection)
	logSources := []*sources.LogSource{
		sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/1/*.log", suite.testDir)}),
		sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/2/*.log", suite.testDir)}),
	}
	status.InitStatus(util.CreateSources(logSources))
	files := fileProvider.FilesToTail(logSources)
	suite.Equal(2, len(files))
	suite.Equal([]string{"2 files tailed out of 3 files matching"}, logSources[0].Messages.GetMessages())
	suite.Equal(
		[]string{
			"The limit on the maximum number of files in use (2) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		},
		status.Get().Warnings,
	)
	suite.Equal([]string{"0 files tailed out of 2 files matching"}, logSources[1].Messages.GetMessages())
	suite.Equal(
		[]string{
			"The limit on the maximum number of files in use (2) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		},
		status.Get().Warnings,
	)

	os.Remove(fmt.Sprintf("%s/1/2.log", suite.testDir))
	os.Remove(fmt.Sprintf("%s/1/3.log", suite.testDir))
	os.Remove(fmt.Sprintf("%s/2/2.log", suite.testDir))
	files = fileProvider.FilesToTail(logSources)
	suite.Equal(2, len(files))
	suite.Equal([]string{"1 files tailed out of 1 files matching"}, logSources[0].Messages.GetMessages())

	suite.Equal([]string{"1 files tailed out of 1 files matching"}, logSources[1].Messages.GetMessages())
	suite.Equal(
		[]string{
			"The limit on the maximum number of files in use (2) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		},
		status.Get().Warnings,
	)

	os.Remove(fmt.Sprintf("%s/2/1.log", suite.testDir))

	files = fileProvider.FilesToTail(logSources)
	suite.Equal(1, len(files))
	suite.Equal([]string{"1 files tailed out of 1 files matching"}, logSources[0].Messages.GetMessages())

	suite.Equal([]string{"0 files tailed out of 0 files matching"}, logSources[1].Messages.GetMessages())
}

func (suite *ProviderTestSuite) TestExcludePath() {
	filesLimit := 6
	path := fmt.Sprintf("%s/*/*.log", suite.testDir)
	excludePaths := []string{fmt.Sprintf("%s/2/*.log", suite.testDir)}
	fileProvider := NewFileProvider(filesLimit, SortReverseLexicographical, GreedySelection)
	logSources := []*sources.LogSource{
		sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path, ExcludePaths: excludePaths}),
	}

	files := fileProvider.FilesToTail(logSources)
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
		fileProvider := NewFileProvider(2, SortReverseLexicographical, GreedySelection)
		source := sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: "//\\///*"})
		files, err := fileProvider.CollectFiles(source)
		assert.Len(t, files, 0)
		assert.Error(t, err)
	})
	t.Run("ReverseLexicographical", func(t *testing.T) {
		testDir := t.TempDir()
		path := func(name string) string {
			return fmt.Sprintf("%s/%s", testDir, name)
		}
		createFile := func(name string) {
			_, err := os.Create(path(name))
			assert.Nil(t, err)
		}
		createFile("a")
		createFile("b")
		createFile("c")
		createFile("d")

		fileProvider := NewFileProvider(2, SortReverseLexicographical, GreedySelection)
		source := sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: path("*")})
		files, err := fileProvider.CollectFiles(source)
		assert.Nil(t, err)
		assert.Len(t, files, 4)
		assert.Equal(t, path("d"), files[0].Path)
		assert.Equal(t, path("c"), files[1].Path)
		assert.Equal(t, path("b"), files[2].Path)
		assert.Equal(t, path("a"), files[3].Path)
	})

	t.Run("Mtime", func(t *testing.T) {
		testDir := t.TempDir()
		baseTime := time.Date(2010, time.August, 10, 25, 0, 0, 0, time.UTC)

		path := func(name string) string {
			return fmt.Sprintf("%s/%s", testDir, name)
		}
		createFile := func(name string, time time.Time) {
			_, err := os.Create(path(name))
			assert.Nil(t, err)
			err = os.Chtimes(path(name), time, time)
			assert.Nil(t, err)
		}

		// Given 4 files with descending mtimes
		createFile("a.log", baseTime.Add(time.Second*4))
		createFile("q.log", baseTime.Add(time.Second*3))
		createFile("t.log", baseTime.Add(time.Second*2))
		createFile("z.log", baseTime.Add(time.Second*1))

		fileProvider := NewFileProvider(2, SortMtime, GreedySelection)
		source := sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: path("*")})
		files, err := fileProvider.CollectFiles(source)
		assert.Nil(t, err)
		assert.Len(t, files, 4)
		assert.Equal(t, path("a.log"), files[0].Path)
		assert.Equal(t, path("q.log"), files[1].Path)
		assert.Equal(t, path("t.log"), files[2].Path)
		assert.Equal(t, path("z.log"), files[3].Path)
	})
}

func TestFilesToTail(t *testing.T) {
	t.Run("Reverse Lexicographical - Greedy", func(t *testing.T) {
		testDir := t.TempDir()
		path := func(name string) string {
			return fmt.Sprintf("%s/%s", testDir, name)
		}
		createFile := func(name string) {
			_, err := os.Create(path(name))
			assert.Nil(t, err)
		}
		mkDir := func(name string) {
			err := os.Mkdir(path(name), os.ModePerm)
			assert.Nil(t, err)
		}

		mkDir("a")
		createFile("a/a")
		createFile("a/b")
		createFile("a/z")
		mkDir("b")
		createFile("b/a")
		createFile("b/b")
		createFile("b/z")

		fileProvider := NewFileProvider(2, SortReverseLexicographical, GreedySelection)
		sources := []*sources.LogSource{
			sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: path("a/*")}),
			sources.NewLogSource("wildcardTwo", &config.LogsConfig{Type: config.FileType, Path: path("b/*")}),
		}
		files := fileProvider.FilesToTail(sources)
		assert.Len(t, files, 2)
		assert.Equal(t, path("a/z"), files[0].Path)
		assert.Equal(t, path("a/b"), files[1].Path)
	})
	t.Run("Reverse Lexicographical - Global", func(t *testing.T) {
		t.Skip()
		testDir := t.TempDir()
		path := func(name string) string {
			return fmt.Sprintf("%s/%s", testDir, name)
		}
		createFile := func(name string) {
			_, err := os.Create(path(name))
			assert.Nil(t, err)
		}
		mkDir := func(name string) {
			err := os.Mkdir(path(name), os.ModePerm)
			assert.Nil(t, err)
		}

		mkDir("a")
		createFile("a/a")
		createFile("a/b")
		createFile("a/z")
		mkDir("b")
		createFile("b/a")
		createFile("b/b")
		createFile("b/z")

		fileProvider := NewFileProvider(2, SortReverseLexicographical, GlobalSelection)
		sources := []*sources.LogSource{
			sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: path("a/*")}),
			sources.NewLogSource("wildcardTwo", &config.LogsConfig{Type: config.FileType, Path: path("b/*")}),
		}
		files := fileProvider.FilesToTail(sources)
		assert.Len(t, files, 2)
		assert.Equal(t, path("b/z"), files[0].Path)
		assert.Equal(t, path("b/b"), files[1].Path)
	})
	t.Run("Mtime - Greedy", func(t *testing.T) {
		testDir := t.TempDir()
		baseTime := time.Date(2010, time.August, 10, 25, 0, 0, 0, time.UTC)

		path := func(name string) string {
			return fmt.Sprintf("%s/%s", testDir, name)
		}
		createFile := func(name string, time time.Time) {
			_, err := os.Create(path(name))
			assert.Nil(t, err)
			err = os.Chtimes(path(name), time, time)
			assert.Nil(t, err)
		}

		// Given 4 files with descending mtimes
		createFile("a.log", baseTime.Add(time.Second*4))
		createFile("q.log", baseTime.Add(time.Second*3))
		createFile("t.log", baseTime.Add(time.Second*2))
		createFile("z.log", baseTime.Add(time.Second*1))
		// TODO test behavior when time is equal

		fileProvider := NewFileProvider(2, SortMtime, GreedySelection)
		sources := []*sources.LogSource{
			sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: path("*")}),
		}
		files := fileProvider.FilesToTail(sources)
		assert.Len(t, files, 2)
		assert.Equal(t, path("a.log"), files[0].Path)
		assert.Equal(t, path("q.log"), files[1].Path)
	})
	t.Run("Mtime - Global", func(t *testing.T) {
		t.Skip()
		testDir := t.TempDir()
		baseTime := time.Date(2010, time.August, 10, 25, 0, 0, 0, time.UTC)

		path := func(name string) string {
			return fmt.Sprintf("%s/%s", testDir, name)
		}
		createFile := func(name string, time time.Time) {
			_, err := os.Create(path(name))
			assert.Nil(t, err)
			err = os.Chtimes(path(name), time, time)
			assert.Nil(t, err)
		}
		mkDir := func(name string) {
			err := os.Mkdir(path(name), os.ModePerm)
			assert.Nil(t, err)
		}

		// Given 4 files with descending mtimes
		mkDir("a")
		createFile("a/a", baseTime.Add(time.Second*4))
		createFile("a/b", baseTime.Add(time.Second*3))
		createFile("a/c", baseTime.Add(time.Second*8))
		mkDir("b")
		createFile("b/a", baseTime.Add(time.Second*6))
		createFile("b/b", baseTime.Add(time.Second*7))
		createFile("b/c", baseTime.Add(time.Second*8))

		fileProvider := NewFileProvider(2, SortMtime, GreedySelection)
		sources := []*sources.LogSource{
			sources.NewLogSource("wildcard", &config.LogsConfig{Type: config.FileType, Path: path("a/*")}),
			sources.NewLogSource("wildcardTwo", &config.LogsConfig{Type: config.FileType, Path: path("b/*")}),
		}
		files := fileProvider.FilesToTail(sources)
		assert.Len(t, files, 2)
		assert.Equal(t, path("a/c"), files[0].Path)
		assert.Equal(t, path("b/c"), files[1].Path)
	})
}

func BenchmarkApplyOrdering(b *testing.B) {
	b.Run("Mtime", func(b *testing.B) {
		testDir := b.TempDir()
		baseTime := time.Date(2010, time.August, 25, 0, 0, 0, 0, time.UTC)

		path := func(name string) string {
			return fmt.Sprintf("%s/%s", testDir, name)
		}
		createFile := func(name string, time time.Time) {
			_, err := os.Create(path(name))
			assert.Nil(b, err)
			err = os.Chtimes(path(name), time, time)
			assert.Nil(b, err)
		}

		createFile("a.log", baseTime.Add(time.Second*4))
		createFile("b.log", baseTime.Add(time.Second*2))
		createFile("c.log", baseTime.Add(time.Second*5))
		createFile("d.log", baseTime.Add(time.Second*5))

		fileProvider := NewFileProvider(2, SortMtime, GreedySelection)
		files := []string{
			path("a.log"),
			path("b.log"),
			path("c.log"),
			path("d.log"),
		}
		for n := 0; n < b.N; n++ {
			fileProvider.applyOrdering(files)
		}
	})

	b.Run("ReverseLexicographical", func(b *testing.B) {
		testDir := b.TempDir()
		baseTime := time.Date(2010, time.August, 25, 0, 0, 0, 0, time.UTC)

		path := func(name string) string {
			return fmt.Sprintf("%s/%s", testDir, name)
		}
		createFile := func(name string, time time.Time) {
			_, err := os.Create(path(name))
			assert.Nil(b, err)
			err = os.Chtimes(path(name), time, time)
			assert.Nil(b, err)
		}

		createFile("a.log", baseTime.Add(time.Second*4))
		createFile("b.log", baseTime.Add(time.Second*2))
		createFile("c.log", baseTime.Add(time.Second*5))
		createFile("d.log", baseTime.Add(time.Second*5))

		fileProvider := NewFileProvider(2, SortReverseLexicographical, GreedySelection)
		files := []string{
			path("a.log"),
			path("b.log"),
			path("c.log"),
			path("d.log"),
		}
		for n := 0; n < b.N; n++ {
			fileProvider.applyOrdering(files)
		}
	})

}

func TestApplyOrdering(t *testing.T) {
	t.Run("Mtime", func(t *testing.T) {
		testDir := t.TempDir()
		baseTime := time.Date(2010, time.August, 25, 0, 0, 0, 0, time.UTC)

		path := func(name string) string {
			return fmt.Sprintf("%s/%s", testDir, name)
		}
		createFile := func(name string, time time.Time) {
			_, err := os.Create(path(name))
			assert.Nil(t, err)
			err = os.Chtimes(path(name), time, time)
			assert.Nil(t, err)
		}

		// Given 4 files with descending mtimes
		createFile("a.log", baseTime.Add(time.Second*4))
		createFile("t.log", baseTime.Add(time.Second*2))
		createFile("q.log", baseTime.Add(time.Second*3))
		createFile("z.log", baseTime.Add(time.Second*1))

		fileProvider := NewFileProvider(2, SortMtime, GreedySelection)
		files := []string{
			path("t.log"),
			path("a.log"),
			path("z.log"),
			path("q.log"),
		}
		// When we apply ordering
		fileProvider.applyOrdering(files)
		// Then we should see all files in descending mtime order
		assert.Len(t, files, 4)
		assert.Equal(t, path("a.log"), files[0])
		assert.Equal(t, path("q.log"), files[1])
		assert.Equal(t, path("t.log"), files[2])
		assert.Equal(t, path("z.log"), files[3])
	})

	t.Run("Reverse Lexicographical", func(t *testing.T) {
		fileProvider := NewFileProvider(0, SortReverseLexicographical, GreedySelection)
		// For lexicographical ordering, we don't actually need the files
		// to exist on the FS, so that part is skipped for these tests
		t.Run("Flat Directory", func(t *testing.T) {
			files := []string{
				"a.log",
				"t.log",
				"q.log",
				"z.log",
			}
			fileProvider.applyOrdering(files)

			assert.Len(t, files, 4)
			assert.Equal(t, "z.log", files[0])
			assert.Equal(t, "t.log", files[1])
			assert.Equal(t, "q.log", files[2])
			assert.Equal(t, "a.log", files[3])
		})
		t.Run("Multiple Directories dated log file", func(t *testing.T) {
			paths := []string{
				"/tmp/1/2018.log",
				"/tmp/1/2017.log",
				"/tmp/2/2018.log",
				"/tmp/1/2016.log",
			}
			fileProvider.applyOrdering(paths)
			assert.Equal(t, "/tmp/2/2018.log", paths[0])
			assert.Equal(t, "/tmp/1/2018.log", paths[1])
			assert.Equal(t, "/tmp/1/2017.log", paths[2])
			assert.Equal(t, "/tmp/1/2016.log", paths[3])
		})

		t.Run("Multiple Directories - dated directory", func(t *testing.T) {
			paths := []string{
				"/tmp/2020-02-20/error.log",
				"/tmp/2020-02-21/error.log",
				"/tmp/2020-02-22/error.log",
			}
			fileProvider.applyOrdering(paths)
			assert.Equal(t, "/tmp/2020-02-22/error.log", paths[0])
			assert.Equal(t, "/tmp/2020-02-21/error.log", paths[1])
			assert.Equal(t, "/tmp/2020-02-20/error.log", paths[2])
		})
		t.Run("Multiple Directories - Out of order input", func(t *testing.T) {
			t.Skip() // See FIXME in 'applyOrdering', this test currently fails
			paths := []string{
				"/tmp/1/2018.log",
				"/tmp/2/2018.log",
				"/tmp/3/2016.log",
				"/tmp/3/2018.log",
				"/tmp/1/2017.log",
			}
			fileProvider.applyOrdering(paths)

			assert.Equal(t, "/tmp/3/2018.log", paths[0])
			assert.Equal(t, "/tmp/3/2016.log", paths[1])
			assert.Equal(t, "/tmp/2/2018.log", paths[2])
			assert.Equal(t, "/tmp/1/2018.log", paths[3])
			assert.Equal(t, "/tmp/1/2017.log", paths[4])
		})
	})
}
