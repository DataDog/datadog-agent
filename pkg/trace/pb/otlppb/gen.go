//go:generate protoc --gogo_out=plugins=grpc:. trace.proto resource.proto common.proto trace_service.proto
//go:generate protoc --grpc-gateway_out=logtostderr=true:. trace_service.proto

package otlppb
