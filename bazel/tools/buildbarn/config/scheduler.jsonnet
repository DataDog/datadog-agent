local common = import 'common.libsonnet';

{
  adminHttpServers: [{
    listenAddresses: [':7982'],
    authenticationPolicy: common.allow,
  }],
  clientGrpcServers: [{
    listenAddresses: [':8982'],
    authenticationPolicy: common.allow,
  }],
  workerGrpcServers: [{
    listenAddresses: [':8983'],
    authenticationPolicy: common.allow,
  }],
  contentAddressableStorage: common.storage,
  maximumMessageSizeBytes: common.maximumMessageSizeBytes,
  executeAuthorizer: common.allow,
  modifyDrainsAuthorizer: common.allow,
  killOperationsAuthorizer: common.allow,
  synchronizeAuthorizer: common.allow,
  actionRouter: {
    simple: {
      platformKeyExtractor: { action: {} },
      invocationKeyExtractors: [
        { correlatedInvocationsId: {} },
        { toolInvocationId: {} },
      ],
      initialSizeClassAnalyzer: {
        defaultExecutionTimeout: '1800s',
        maximumExecutionTimeout: '7200s',
      },
    },
  },
  platformQueueWithNoWorkersTimeout: '900s',
}
