// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type FileManager struct {
	runner  Runner
	command OSCommand
}

func NewFileManager(runner Runner) *FileManager {
	return &FileManager{
		runner:  runner,
		command: runner.OsCommand(),
	}
}

func (fm *FileManager) IsPathAbsolute(path string) bool {
	return fm.command.IsPathAbsolute(path)
}

// CreateDirectoryFromPulumiString if it does not exist from directory name as a Pulumi String
func (fm *FileManager) CreateDirectoryFromPulumiString(name string, remotePath pulumi.String, useSudo bool, opts ...pulumi.ResourceOption) (Command, error) {
	return fm.command.CreateDirectory(fm.runner, name, remotePath, useSudo, opts...)
}

// CreateDirectoryForFile if the directory does not exist
// To avoid pulumi.URN collisions if multiple files use the same directory, use the full filePath as URN and path.Split out the folderPath for creation
func (fm *FileManager) CreateDirectoryForFile(remotePath string, useSudo bool, opts ...pulumi.ResourceOption) (Command, error) {
	// if given just a directory path, path.Split returns "" as file
	//  eg. path.Split("/a/b/c/") -> "/a/b/c/", ""
	folderPath, _ := path.Split(remotePath)
	return fm.command.CreateDirectory(fm.runner, "create-directory-"+remotePath, pulumi.String(folderPath), useSudo, opts...)
}

// CreateDirectory if it does not exist
func (fm *FileManager) CreateDirectory(remotePath string, useSudo bool, opts ...pulumi.ResourceOption) (Command, error) {
	return fm.command.CreateDirectory(fm.runner, "create-directory-"+remotePath, pulumi.String(remotePath), useSudo, opts...)
}

// TempDirectory creates a temporary directory
func (fm *FileManager) TempDirectory(folderName string, opts ...pulumi.ResourceOption) (Command, string, error) {
	tempDir := path.Join(fm.command.GetTemporaryDirectory(), folderName)
	folderCmd, err := fm.CreateDirectory(tempDir, false, opts...)
	return folderCmd, tempDir, err
}

// HomeDirectory creates a directory in home directory, if it does not exist
// A home directory is a file system directory on a multi-user operating system containing files for a given user of the system.
// It does not require sudo, using sudo in home directory allows to change default ownership and it is discouraged.
func (fm *FileManager) HomeDirectory(folderName string, opts ...pulumi.ResourceOption) (Command, string, error) {
	homeDir := path.Join(fm.command.GetHomeDirectory(), folderName)
	folderCmd, err := fm.CreateDirectory(homeDir, false, opts...)
	return folderCmd, homeDir, err
}

func (fm *FileManager) CopyFile(name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return fm.runner.newCopyFile(name, localPath, remotePath, opts...)
}

// CopyToRemoteFile copies a local file to a remote file. Under the hood it uses remote.CopyToRemote, so localPath will be converted to a File Asset, it breaks if the local path is not known at plan time.
// Ideally it should replace CopyFile but it is not possible due to the limitation of CopyToRemote for now.
func (fm *FileManager) CopyToRemoteFile(name string, localPath, remotePath pulumi.StringInput, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	return fm.command.NewCopyToRemoteFile(fm.runner, name, localPath, remotePath, opts...)
}

func (fm *FileManager) CopyInlineFile(fileContent pulumi.StringInput, remotePath string, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	// Write the content into a temporary file and get the path

	localTempPath := fileContent.ToStringOutput().ApplyT(func(content string) (string, error) {
		tempFile, err := os.CreateTemp("", filepath.Base(remotePath))
		if err != nil {
			return "", err
		}

		if err != nil {
			return "", err
		}
		defer tempFile.Close()

		tempFilePath := tempFile.Name()
		_, err = tempFile.WriteString(content)
		if err != nil {
			return "", err
		}
		return tempFilePath, nil
	}).(pulumi.StringInput)

	return fm.CopyFile(remotePath, localTempPath, pulumi.String(remotePath), opts...)
}

// CopyRelativeFolder copies recursively a relative folder to a remote folder.
// The path of the folder is relative to the caller of this function.
// For example, if this function is called from ~/foo/test.go with remoteFolder="bar"
// then the full path of the folder will be ~/foo/bar“.
// This function returns the resources that can be used with `pulumi.DependsOn`.
func (fm *FileManager) CopyRelativeFolder(relativeFolder string, remoteFolder string, opts ...pulumi.ResourceOption) ([]pulumi.Resource, error) {
	// `./` cannot be used with os.DirFS
	relativeFolder = strings.TrimPrefix(relativeFolder, "."+string(filepath.Separator))

	_, rootFolder, err := getFullPath(relativeFolder, 2)
	if err != nil {
		return nil, err
	}

	return fm.CopyFSFolder(os.DirFS(rootFolder), relativeFolder, remoteFolder, opts...)
}

// CopyAbsoluteFolder copies recursively a folder to a remote folder.
// This function returns the resources that can be used with `pulumi.DependsOn`.
func (fm *FileManager) CopyAbsoluteFolder(absoluteFolder string, remoteFolder string, opts ...pulumi.ResourceOption) ([]pulumi.Resource, error) {
	baseFolder := filepath.Base(absoluteFolder)
	rootWithoutBase := absoluteFolder[:len(absoluteFolder)-len(baseFolder)]
	// Use remoteFolder as `absoluteFolder` may be a random file path that is different for each run.
	return fm.CopyFSFolder(os.DirFS(rootWithoutBase), baseFolder, remoteFolder, opts...)
}

// CopyRelativeFile copies relative path to a remote path.
// The relative path is defined in the same way as for `CopyRelativeFolder`.
// This function returns the resource that can be used with `pulumi.DependsOn`.
func (fm *FileManager) CopyRelativeFile(relativePath string, remotePath string, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	fullPath, _, err := getFullPath(relativePath, 2)
	if err != nil {
		return nil, err
	}

	return fm.CopyFile(filepath.Base(relativePath), pulumi.String(fullPath), pulumi.String(remotePath), opts...)
}

// CopyFSFolder copies recursively a local folder to a remote folder.
// You may consider using `CopyRelativeFolder` which has a simpler API.
func (fm *FileManager) CopyFSFolder(
	folderFS fs.FS,
	folderPath string,
	remoteFolder string,
	opts ...pulumi.ResourceOption,
) ([]pulumi.Resource, error) {
	useSudo := true
	folderCommand, err := fm.CreateDirectory(remoteFolder, useSudo, opts...)
	if err != nil {
		return nil, fmt.Errorf("cannot create a temporary folder: %v for resource %v", err, remoteFolder)
	}

	files, folders, err := readFilesAndFolders(folderFS, folderPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read files and folders for %v. Error: %v", folderPath, err)
	}

	var folderResources []pulumi.Resource
	for _, folder := range folders {
		destFolder, err := getDestinationPath(folder, folderPath)
		if err != nil {
			return nil, err
		}
		remotePath := path.Join(remoteFolder, destFolder)
		resources, err := fm.CreateDirectory(remotePath, useSudo, pulumi.DependsOn([]pulumi.Resource{folderCommand}))
		if err != nil {
			return nil, err
		}
		folderResources = append(folderResources, resources)
	}

	fileResources := []pulumi.Resource{}
	for _, file := range files {
		destFile, err := getDestinationPath(file, folderPath)
		if err != nil {
			return nil, err
		}

		fileContent, err := fs.ReadFile(folderFS, file)
		if err != nil {
			return nil, err
		}
		fileCommand, err := fm.CopyInlineFile(
			pulumi.String(fileContent),
			path.Join(remoteFolder, destFile),
			pulumi.DependsOn(folderResources))
		if err != nil {
			return nil, err
		}
		fileResources = append(fileResources, fileCommand)
	}

	return fileResources, nil
}

// When copying foo/bar to /tmp the result folder is /tmp/bar
// This function remove the root prefix from the path (`foo` in this case)
func getDestinationPath(folder string, rootFolder string) (string, error) {
	baseFolder := filepath.Base(rootFolder)
	rootWithoutBase := rootFolder[:len(rootFolder)-len(baseFolder)]

	if !strings.HasPrefix(folder, rootWithoutBase) {
		return "", fmt.Errorf("%v doesn't have the prefix %v", folder, rootWithoutBase)
	}

	return folder[len(rootWithoutBase):], nil
}

func getFullPath(relativeFolder string, skip int) (string, string, error) {
	_, file, _, ok := runtime.Caller(skip)
	if !ok {
		return "", "", errors.New("cannot get the runtime caller path")
	}
	folder := filepath.Dir(file)
	fullPath := path.Join(folder, relativeFolder)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", "", fmt.Errorf("the path %v doesn't exist. Caller folder: %v, relative folder: %v", fullPath, folder, relativeFolder)
	}
	return fullPath, folder, nil
}

func readFilesAndFolders(folderFS fs.FS, folderPath string) ([]string, []string, error) {
	var files []string
	var folders []string
	err := fs.WalkDir(folderFS, folderPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			folders = append(folders, path)
		} else {
			files = append(files, path)
		}
		return nil
	})

	return files, folders, err
}
