{
  allow: { allow: {} },
  storage: { grpc: { client: { address: 'storage:8980' } } },
  maximumMessageSizeBytes: 2 * 1024 * 1024,
}
