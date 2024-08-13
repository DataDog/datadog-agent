// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lsof

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

func listOpenFiles(ctx context.Context, pid int) (Files, error) {
	p, err := process.NewProcessWithContext(ctx, int32(pid))
	if err != nil {
		return nil, err
	}

	openFiles, err := p.OpenFilesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	var files Files
	for _, openFile := range openFiles {
		fd := fmt.Sprintf("%d", openFile.Fd)
		file, err := getFileInfo(ctx, openFile.Path, fd)
		if err == nil {
			files = append(files, file)
		}
	}

	maps, err := p.MemoryMapsWithContext(ctx, false)
	if err != nil {
		return nil, err
	}

	for _, m := range *maps {
		file, err := getFileInfo(ctx, m.Path, "unknown")
		if err == nil {
			files = append(files, file)
		}
	}

	return files, nil
}

func getFileInfo(_ context.Context, path string, fd string) (File, error) {
	ty := "unknown"
	perm := "unknown"
	var size int64

	if strings.HasPrefix(path, "[") {
		ty = "a_inode"
	} else if s, err := os.Stat(path); err == nil {
		if s.IsDir() {
			ty = "DIR"
		} else if s.Mode().IsRegular() {
			ty = "REG"
		}
		perm = s.Mode().Perm().String()
		size = s.Size()
	}

	return File{
		Fd:   fd,
		Type: ty,
		Perm: perm,
		Size: size,
		Name: path,
	}, nil
}
