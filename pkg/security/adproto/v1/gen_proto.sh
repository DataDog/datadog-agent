#!/bin/bash
set -euxo pipefail

# go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

protoc -I. \
    --go_out=paths=source_relative:. \
    --go-vtproto_out=. --plugin protoc-gen-go-vtproto="${GOPATH}/bin/protoc-gen-go-vtproto" \
    --go-vtproto_opt=features=pool+marshal+size \
    --go-vtproto_opt=pool=pkg/security/adproto/v1.ActivityDump \
    --go-vtproto_opt=pool=pkg/security/adproto/v1.ProcessActivityNode \
    --go-vtproto_opt=pool=pkg/security/adproto/v1.FileActivityNode \
    --go-vtproto_opt=pool=pkg/security/adproto/v1.FileInfo \
    --go-vtproto_opt=pool=pkg/security/adproto/v1.ProcessInfo \
    pkg/security/adproto/v1/activity_dump.proto
