local common = import 'common.libsonnet';

{
  buildDirectoryPath: '/worker/build',
  grpcServers: [{
    listenPaths: ['/worker/runner'],
    authenticationPolicy: common.allow,
  }],
}
