# OTLP Metric Exporter Sample App

This sample application exports 1 count metric, 1 sum metric and 1 histogram metric via OTLP metric exporter.
It can be configured to export to either Datadog Agent OTLP metric intake or OpenTelemtry Collector OTLP receiver.

## Usage

```
go build .
./metric [http | grpc] [endpoint] [delta | cumulative]
```

All arguments are optional. 
- The 1st arg specifies which protocol to use, default is gRPC.
- The 2nd arg specifies the metric intake endpoint. Default is `localhost:4317` for gRPC and `localhost:4318` for HTTP.
- The 3rd arg specifies the metric aggregation temporality, default is `cumulative`.