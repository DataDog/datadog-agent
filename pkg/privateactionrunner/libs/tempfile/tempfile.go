// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tempfile

import (
	"encoding/json"
	"io"
	"os"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
)

type TempFile struct {
	*os.File
}

func New(postfix string) (*TempFile, error) {
	file, err := os.CreateTemp("", postfix)
	if err != nil {
		return nil, err
	}
	return &TempFile{File: file}, nil
}

func NewWithContent(postfix string, content []byte) (*TempFile, error) {
	tf, err := New(postfix)
	if err != nil {
		return nil, err
	}
	_, err = tf.Write(content)
	if err != nil {
		return nil, err
	}
	return tf, nil
}

func (tf *TempFile) ReadJSON() (data any, err error) {
	decoder := json.NewDecoder(tf)
	err = decoder.Decode(&data)
	return data, err
}

func (tf *TempFile) ReadBytes() ([]byte, error) {
	return io.ReadAll(tf.File)
}

func (tf *TempFile) Close() (err error) {
	err = tf.File.Close()
	if err != nil {
		return err
	}
	return os.Remove(tf.File.Name())
}

func (tf *TempFile) CloseSafely() {
	err := tf.Close()
	if err != nil {
		log.Warn("could not close temp file", log.String("file_name", tf.File.Name()), log.ErrorField(err))
	}
}
