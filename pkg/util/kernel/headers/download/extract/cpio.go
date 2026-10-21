// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package extract

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sassoftware/go-rpmutils"
	"github.com/sassoftware/go-rpmutils/cpio"
)

// openRPMPayload retains the compression formats supported by rpmutils, but owns
// the decompressor so errors during extraction cannot leak decoder resources.
func openRPMPayload(reader io.Reader, header *rpmutils.RpmHeader) (io.ReadCloser, error) {
	if header.HasTag(rpmutils.PAYLOADFORMAT) {
		format, err := header.GetString(rpmutils.PAYLOADFORMAT)
		if err != nil {
			return nil, err
		}
		if format != "cpio" {
			return nil, fmt.Errorf("unsupported RPM payload format %q", format)
		}
	}
	compression := compressionNone
	if header.HasTag(rpmutils.PAYLOADCOMPRESSOR) {
		name, err := header.GetString(rpmutils.PAYLOADCOMPRESSOR)
		if err != nil {
			return nil, err
		}
		switch name {
		case "gzip":
			compression = compressionGzip
		case "zstd":
			compression = compressionZstd
		case "bzip2":
			compression = compressionBzip2
		case "xz":
			compression = compressionXZ
		case "lzma":
			compression = compressionLZMA
		case "uncompressed":
			compression = compressionNone
		default:
			return nil, fmt.Errorf("unsupported RPM compression %q", name)
		}
	} else {
		buffered := bufio.NewReader(reader)
		magic, err := buffered.Peek(2)
		if err != nil {
			return nil, err
		}
		reader = buffered
		if magic[0] == 0x1f && magic[1] == 0x8b {
			compression = compressionGzip
		}
	}
	return openCompressed(reader, compression)
}

// A valid CPIO ends with a trailer, for which cpio.Reader.Next returns io.EOF.
// An underlying EOF before that trailer must instead be treated as truncation.
type cpioInput struct{ io.Reader }

func (r cpioInput) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

func extractRPMPayload(payload io.ReadCloser, destination *extractionDirectory) error {
	defer payload.Close()
	if err := extractCPIO(payload, destination); err != nil {
		return err
	}
	// Consume trailing padding and allow the decoder to check its stream checksum.
	_, err := io.CopyBuffer(io.Discard, payload, destination.buffer)
	return err
}

type cpioInode struct{ deviceMajor, deviceMinor, inode int }
type cpioLinks struct {
	names   []string
	content string
	mode    int64
}

func extractCPIO(payload io.Reader, destination *extractionDirectory) error {
	reader := cpio.NewReader(cpioInput{payload})
	// All names sharing a CPIO device and inode are hardlinks. The archive can
	// put their content on any member, so retain the names until it is found.
	groups := make(map[cpioInode]*cpioLinks)
	for {
		hdr, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read CPIO entry: %w", err)
		}
		name, err := archivePath(hdr.Filename())
		if err != nil {
			return err
		}
		mode := hdr.Mode()
		switch mode &^ 07777 {
		case cpio.S_ISDIR:
			if hdr.Filesize64() != 0 {
				return fmt.Errorf("directory %q has a payload", name)
			}
			err = destination.mkdir(name)
		case cpio.S_ISLNK:
			if hdr.Filesize64() < 1 || hdr.Filesize64() > 4096 {
				return fmt.Errorf("invalid symlink size for %q", name)
			}
			target := make([]byte, hdr.Filesize64())
			if _, err = io.ReadFull(reader, target); err == nil {
				err = destination.symlink(name, string(target))
			}
		case cpio.S_ISREG:
			if hdr.Nlink() > 1 {
				id := cpioInode{hdr.Devmajor(), hdr.Devminor(), hdr.Ino()}
				group := groups[id]
				if group == nil {
					group = &cpioLinks{mode: int64(mode)}
					groups[id] = group
				}
				group.names = append(group.names, name)
				if hdr.Filesize64() == 0 {
					// This member has no data; another member may carry it.
					continue
				}
				if group.content != "" {
					return fmt.Errorf("multiple payloads for CPIO hardlink group at %q", name)
				}
				group.content = name
			}
			limited := &io.LimitedReader{R: reader, N: hdr.Filesize64()}
			err = destination.write(name, int64(mode), limited)
			if err == nil && limited.N != 0 {
				err = io.ErrUnexpectedEOF
			}
		default:
			err = fmt.Errorf("unsupported CPIO file mode %#o", mode)
		}
		if err != nil {
			return fmt.Errorf("extract CPIO entry %q: %w", name, err)
		}
	}
	// Create links only after extracting the content-bearing member. If every
	// member was empty, create one empty file to serve as the link target.
	for _, group := range groups {
		if group.content == "" {
			group.content = group.names[0]
			if err := destination.write(group.content, group.mode, strings.NewReader("")); err != nil {
				return err
			}
		}
		for _, name := range group.names {
			if name != group.content {
				if err := destination.hardlink(name, group.content); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// CPIO hardlinks refer directly to a regular member of the same inode group.
func (destination *extractionDirectory) hardlink(name, target string) error {
	if err := destination.mkdir(filepath.Dir(name)); err != nil {
		return err
	}
	fi, err := destination.root.Lstat(target)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("hardlink target %q is not a regular file", target)
	}
	if existing, err := destination.root.Lstat(name); err == nil && os.SameFile(fi, existing) {
		return nil
	}
	return destination.root.Link(target, name)
}
