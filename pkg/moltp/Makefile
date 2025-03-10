v1/metrics_service.pb.go v1/metrics_service_grpc.pb.go: v1 proto/metrics_service.proto
	git clone https://github.com/open-telemetry/opentelemetry-proto.git -b v1.5.0
	protoc --go_out=v1 --go-grpc_out=v1 --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative -I opentelemetry-proto -I proto proto/metrics_service.proto
v1:
	mkdir $@
