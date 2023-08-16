# Application Server

## Purpose

Simulates a relatively busy application server on which DogStatsD metrics,
traces and TCP streamed logs are present on which the client user is interested
in OTel but has not made a complete transition. DogStatsD, TCP logs, and APM
traces make up the load. We make claims about throughput, UDS packet loss and
memory, CPU resource consumption.
