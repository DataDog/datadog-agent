// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	proto "github.com/golang/protobuf/proto"
)

func main() {
	folder, err := parseArg()
	if err != nil {
		fmt.Println(err)
		return
	}
	if err = dumpRetryFiles(folder); err != nil {
		fmt.Println(err)
	}
}

func parseArg() (string, error) {
	var folder = flag.String("folder", "", "The folder containing `.retry` files.")
	flag.Parse()
	if *folder == "" {
		return "", errors.New("Invalid folder: Usage `./retry_file_dump --folder=/opt/datadog-agent/run/transactions_to_retry/c47da40ac935c8fd5ca1441a5ee3d068/`")
	}
	return *folder, nil
}

func dumpRetryFiles(folder string) error {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.Type().IsRegular() && filepath.Ext(entry.Name()) == ".retry" {
			fmt.Println(entry.Name())
			filePath := path.Join(folder, entry.Name())
			fileContent, err := dumpRetryFile(filePath)
			if err != nil {
				return err
			}
			output := filePath + ".json"
			err = os.WriteFile(output, fileContent, 0600)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func dumpRetryFile(file string) ([]byte, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	collection := HttpTransactionProtoCollection{}

	if err := proto.Unmarshal(content, &collection); err != nil {
		return nil, err
	}
	buffer := bytes.NewBuffer([]byte{})
	var jsonTrs []JSONHTTPTransaction
	for _, v := range collection.Values {
		payload, err := decodePayload(v.Payload, buffer)
		if err != nil {
			return nil, err
		}
		v.Payload = nil // This field is not used as we want a type string to dump in JSON.
		jsonTrs = append(jsonTrs, JSONHTTPTransaction{
			HttpTransactionProto: v,
			Payload:              payload,
		})
	}
	return json.MarshalIndent(jsonTrs, "", "  ")
}

// JSONHTTPTransaction is a transaction object with a string-type Payload associated
// with it
type JSONHTTPTransaction struct {
	*HttpTransactionProto
	Payload string // Same as HttpTransactionProto.Payload but the type is string instead of []byte
}

func decodePayload(payload []byte, buffer *bytes.Buffer) (string, error) {
	buffer.Reset()
	buffer.Write(payload)
	reader, err := zlib.NewReader(buffer)
	if err != nil {
		if err == zlib.ErrHeader {
			return string(payload), nil
		}
		return "", err
	}
	defer reader.Close()

	buff := make([]byte, 200*1000*1000)
	n, err := reader.Read(buff)
	if err != nil && err != io.EOF {
		return "", err
	}
	if n == len(buff) {
		return "", errors.New("Buffer is not big enough")
	}
	return string(buff[:n]), nil
}
