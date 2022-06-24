#!/bin/bash
set -euxo pipefail

# go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

protoc -I. \
    --go_out=paths=source_relative:. \
    --go-vtproto_out=. --plugin protoc-gen-go-vtproto="${GOPATH}/bin/protoc-gen-go-vtproto" \
    pkg/security/adproto/activity_dump.proto
