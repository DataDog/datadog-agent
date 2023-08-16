# OTel Application Server

## Purpose

Simulates a relatively busy application server on which DogStatsD metrics, OTel
traces and TCP streamed logs are present on which the client user has mostly
transitioned to the use of OTeL over DogStatsD and TCP listener logs, although
not entirely replacing these sources of metrics and logs. Traces represent the
majority of load. We make claims about throughput, UDS packet loss and memory,
CPU resource consumption.
