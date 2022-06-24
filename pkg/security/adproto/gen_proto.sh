#!/bin/bash
set -euxo pipefail

# go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

protoc -I. \
    --go_out=paths=source_relative:. \
    --go-vtproto_out=. --plugin protoc-gen-go-vtproto="${GOPATH}/bin/protoc-gen-go-vtproto" \
    --go-vtproto_opt=features=pool+marshal+size \
    --go-vtproto_opt=pool=pkg/security/adproto.ActivityDump \
    --go-vtproto_opt=pool=pkg/security/adproto.ProcessActivityNode \
    --go-vtproto_opt=pool=pkg/security/adproto.FileActivityNode \
    --go-vtproto_opt=pool=pkg/security/adproto.FileInfo \
    --go-vtproto_opt=pool=pkg/security/adproto.ProcessInfo \
    pkg/security/adproto/activity_dump.proto
