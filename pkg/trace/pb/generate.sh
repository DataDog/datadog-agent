#!/usr/bin/env bash


protoc -I. --go_out=paths=source_relative:. --go-vtproto_out=paths=source_relative:. --go-vtproto_opt=features=marshal+unmarshal+size span.proto tracer_payload.proto agent_payload.proto stats.proto
protoc-go-inject-tag -input=span.pb.go
protoc-go-inject-tag -input=tracer_payload.pb.go
protoc-go-inject-tag -input=agent_payload.pb.go

