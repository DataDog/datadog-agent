# Kafka test keys and certificates

The keys and certificates were generated specifically for use by the tests and
are only used from test code.

The JKS files were generated with the `kafka-generate-ssh.sh` script mentioned
in <https://github.com/containers/bitnami/kafka/README.md>. Most fields were left blank.
All passwords were set to `password`.

The client.properties are not used by the tests but are provided for use with
Kafka's console utilities for debugging.

```
kafka-console-producer.sh --bootstrap-server localhost:9093 --producer.config client.properties ...
kafka-console-consumer.sh --bootstrap-server localhost:9093 --consumer.config client.properties ...
```
