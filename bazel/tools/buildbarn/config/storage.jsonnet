// Frontend and storage in one process: clients, the scheduler and the worker
// all talk to this endpoint.
local common = import 'common.libsonnet';

local localBlobstore(dir, sizeBytes, keyLocationMapSizeBytes, newBlocks) = {
  'local': {
    keyLocationMapOnBlockDevice: {
      file: { path: dir + '/key_location_map', sizeBytes: keyLocationMapSizeBytes },
    },
    keyLocationMapMaximumGetAttempts: 16,
    keyLocationMapMaximumPutAttempts: 64,
    oldBlocks: 8,
    currentBlocks: 24,
    newBlocks: newBlocks,
    blocksOnBlockDevice: {
      source: { file: { path: dir + '/blocks', sizeBytes: sizeBytes } },
      spareBlocks: 3,
    },
    persistent: {
      stateDirectoryPath: dir + '/persistent_state',
      minimumEpochInterval: '300s',
    },
  },
};

{
  grpcServers: [{
    listenAddresses: [':8980'],
    authenticationPolicy: common.allow,
  }],
  maximumMessageSizeBytes: common.maximumMessageSizeBytes,
  schedulers: {
    '': { endpoint: { address: 'scheduler:8982' } },
  },
  executeAuthorizer: common.allow,
  contentAddressableStorage: {
    backend: localBlobstore('/storage/cas', 32 * 1024 * 1024 * 1024, 400 * 1024 * 1024, 3),
    getAuthorizer: common.allow,
    putAuthorizer: common.allow,
    findMissingAuthorizer: common.allow,
  },
  actionCache: {
    backend: {
      // Bazel skips downloading intermediate outputs, so cache hits must
      // only be returned while their outputs are still in the CAS.
      completenessChecking: {
        backend: localBlobstore('/storage/ac', 20 * 1024 * 1024, 1024 * 1024, 1),
        maximumTotalTreeSizeBytes: 64 * 1024 * 1024,
      },
    },
    getAuthorizer: common.allow,
    putAuthorizer: common.allow,
  },
}
