// Hardlinking worker: unlike the FUSE one it needs neither privileges nor
// mount propagation, which rootless podman does not offer.
local common = import 'common.libsonnet';

{
  blobstore: {
    contentAddressableStorage: common.storage,
    actionCache: common.storage,
  },
  maximumMessageSizeBytes: common.maximumMessageSizeBytes,
  scheduler: { address: 'scheduler:8983' },
  buildDirectories: [{
    native: {
      // Must share a file system with buildDirectoryPath for hardlinks.
      cacheDirectoryPath: '/worker/cache',
      buildDirectoryPath: '/worker/build',
      maximumCacheFileCount: 200000,
      maximumCacheSizeBytes: 30 * 1024 * 1024 * 1024,
      cacheReplacementPolicy: 'LEAST_RECENTLY_USED',
    },
    runners: [{
      endpoint: { address: 'unix:///worker/runner' },
      concurrency: std.parseInt(std.extVar('BB_CONCURRENCY')),
      // Must exactly match the exec properties sent by Bazel (--remote_default_exec_properties).
      platform: {
        properties: [{ name: 'OSFamily', value: 'linux' }],
      },
      workerId: { hostname: 'dd-buildbarn-worker' },
    }],
  }],
  inputDownloadConcurrency: 10,
  outputUploadConcurrency: 11,
  directoryCache: {
    maximumCount: 1000,
    maximumSizeBytes: 1000 * 1024,
    cacheReplacementPolicy: 'LEAST_RECENTLY_USED',
  },
}
