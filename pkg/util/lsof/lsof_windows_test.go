// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lsof

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"unicode/utf16"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
	"golang.org/x/sys/windows"
)

func TestOpenFiles(t *testing.T) {
	pid := os.Getpid()

	files, err := openFiles(pid)

	require.NoError(t, err)
	require.NotEmpty(t, files)
}

func TestGetDLLFiles(t *testing.T) {
	var procHandle windows.Handle = 42
	modules := []windows.Handle{1, 2, 3}
	paths := map[windows.Handle]string{
		1: "C:/some/path",
		2: "C:/some/other/path",
	}
	stats := map[string]fs.FileInfo{
		paths[1]: &mockFileInfo{
			mode: 0400,
			size: 10,
		},
	}

	ofl := &openFilesLister{
		EnumProcessModules: func(proc windows.Handle, handles *windows.Handle, _ uint32, size *uint32) error {
			require.EqualValues(t, procHandle, proc)

			copySliceToBuff(modules, handles)
			*size = uint32(len(modules)) * uint32(unsafe.Sizeof(procHandle))
			return nil
		},
		GetModuleFileNameEx: func(process, module windows.Handle, buff *uint16, _ uint32) error {
			require.Equal(t, procHandle, process)
			require.Contains(t, modules, module)

			path, ok := paths[module]
			if ok {
				writeStringToUTF16Buffer(path, buff)
				return nil
			}

			return errors.New("some error")
		},
		Stat: func(s string) (os.FileInfo, error) {
			require.Contains(t, maps.Values(paths), s)
			stat, ok := stats[s]
			if ok {
				return stat, nil
			}
			return nil, errors.New("some error")
		},
	}

	expected := Files{
		{
			Fd:       "1",
			Type:     "DLL",
			FilePerm: "-r--------",
			Size:     10,
			Name:     "C:/some/path",
		},
		{
			Fd:       "2",
			Type:     "DLL",
			FilePerm: "<unknown>",
			Size:     -1,
			Name:     "C:/some/other/path",
		},
		{
			Fd:       "3",
			Type:     "DLL",
			FilePerm: "<unknown>",
			Size:     -1,
			Name:     "<error: some error>",
		},
	}

	files, err := ofl.getDLLFiles(procHandle)
	require.NoError(t, err)
	require.ElementsMatch(t, expected, files)
}

func TestGetDLLFilesError(t *testing.T) {
	expectedErr := errors.New("some error")
	ofl := &openFilesLister{
		EnumProcessModules: func(_ windows.Handle, _ *windows.Handle, _ uint32, _ *uint32) error {
			return expectedErr
		},
	}
	_, err := ofl.getDLLFiles(0)
	require.ErrorIs(t, err, expectedErr)
}

func TestGetDLLFile(t *testing.T) {
	t.Run("success with stat", func(t *testing.T) {
		var expectedProc windows.Handle = 42
		var expectedModule windows.Handle = 43
		var expectedSize int64 = 10
		path := "C:/some/path"

		ofl := &openFilesLister{
			GetModuleFileNameEx: func(process, module windows.Handle, buff *uint16, _ uint32) error {
				require.Equal(t, expectedProc, process)
				require.Equal(t, expectedModule, module)
				writeStringToUTF16Buffer(path, buff)
				return nil
			},
			Stat: func(s string) (os.FileInfo, error) {
				require.Equal(t, path, s)
				return &mockFileInfo{
					mode: 0400,
					size: expectedSize,
				}, nil
			},
		}
		file := ofl.getDLLFile(expectedProc, expectedModule)
		expected := File{
			Fd:       fmt.Sprintf("%d", expectedModule),
			FilePerm: "-r--------",
			Name:     path,
			Size:     expectedSize,
			Type:     "DLL",
		}
		require.Equal(t, expected, file)
	})

	t.Run("success without stat", func(t *testing.T) {
		someError := errors.New("some error")
		path := "C:/some/path"
		ofl := &openFilesLister{
			GetModuleFileNameEx: func(_, _ windows.Handle, buff *uint16, _ uint32) error {
				writeStringToUTF16Buffer(path, buff)
				return nil
			},
			Stat: func(s string) (os.FileInfo, error) {
				require.Equal(t, path, s)
				return nil, someError
			},
		}

		file := ofl.getDLLFile(42, 43)
		assert.EqualValues(t, -1, file.Size)
		assert.Equal(t, "<unknown>", file.FilePerm)
	})

	t.Run("path error", func(t *testing.T) {
		someError := errors.New("some error")
		ofl := &openFilesLister{
			GetModuleFileNameEx: func(_, _ windows.Handle, _ *uint16, _ uint32) error {
				return someError
			},
		}
		file := ofl.getDLLFile(42, 43)
		require.Contains(t, file.Name, someError.Error())
		assert.EqualValues(t, -1, file.Size)
		assert.Equal(t, "<unknown>", file.FilePerm)
	})
}

func TestListOpenDLL(t *testing.T) {
	var h windows.Handle
	handleTypeSize := uint32(unsafe.Sizeof(h))

	t.Run("success", func(t *testing.T) {
		var procHandle windows.Handle = 42
		expected := []windows.Handle{1, 2}

		ofl := &openFilesLister{
			EnumProcessModules: func(proc windows.Handle, handles *windows.Handle, _ uint32, size *uint32) error {
				require.EqualValues(t, procHandle, proc)

				copySliceToBuff(expected, handles)
				*size = uint32(len(expected)) * handleTypeSize
				return nil
			},
		}
		handles, err := ofl.listOpenDLL(procHandle)
		require.NoError(t, err)
		require.Equal(t, expected, handles)
	})

	t.Run("success with retry enum", func(t *testing.T) {
		expected := []windows.Handle{1, 2, 3, 4, 5}
		var nbCall int
		var secondCallExpectedSize uint32

		ofl := &openFilesLister{
			EnumProcessModules: func(_ windows.Handle, handles *windows.Handle, byteSize uint32, size *uint32) error {
				require.LessOrEqual(t, nbCall, 1)
				if nbCall == 0 {
					secondCallExpectedSize = byteSize * 2
					*size = secondCallExpectedSize
				} else {
					require.Equal(t, secondCallExpectedSize, byteSize)
					copySliceToBuff(expected, handles)
					*size = uint32(len(expected)) * handleTypeSize
				}

				nbCall++
				return nil
			},
		}

		handles, err := ofl.listOpenDLL(0)
		require.NoError(t, err)
		require.Equal(t, 2, nbCall)
		require.Equal(t, expected, handles)
	})

	t.Run("error", func(t *testing.T) {
		someError := errors.New("some error")

		ofl := &openFilesLister{
			EnumProcessModules: func(_ windows.Handle, _ *windows.Handle, _ uint32, _ *uint32) error {
				return someError
			},
		}
		_, err := ofl.listOpenDLL(0)
		require.ErrorIs(t, err, someError)
	})
}

func TestGetModulePath(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var expectedProc windows.Handle = 42
		var expectedModule windows.Handle = 43
		name := "C:/some/path"

		ofl := &openFilesLister{
			GetModuleFileNameEx: func(proc windows.Handle, module windows.Handle, buff *uint16, _ uint32) error {
				require.Equal(t, expectedProc, proc)
				require.Equal(t, expectedModule, module)
				writeStringToUTF16Buffer(name, buff)
				return nil
			},
		}

		path, err := ofl.getModulePath(expectedProc, expectedModule)
		require.NoError(t, err)
		require.Equal(t, name, path)
	})

	t.Run("error", func(t *testing.T) {
		someError := errors.New("some error")
		ofl := &openFilesLister{
			GetModuleFileNameEx: func(_ windows.Handle, _ windows.Handle, _ *uint16, _ uint32) error {
				return someError
			},
		}

		_, err := ofl.getModulePath(0, 0)
		require.ErrorIs(t, err, someError)
	})
}

// this function assumes the buffer is big enough
func copySliceToBuff[T any](elems []T, buff *T) {
	if len(elems) == 0 {
		return
	}

	for i, v := range elems {
		*(*T)(unsafe.Pointer(uintptr(unsafe.Pointer(buff)) + uintptr(i)*unsafe.Sizeof(elems[0]))) = v
	}
}

// this function assumes the buffer is big enough
func writeStringToUTF16Buffer(str string, buff *uint16) {
	nameUTF16Encoded := utf16.Encode([]rune(str))
	copySliceToBuff(nameUTF16Encoded, buff)
}
