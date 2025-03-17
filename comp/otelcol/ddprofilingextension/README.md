# ddprofiling Extension

The ddprofiling extension allows profiling the collector via the datadog profiler.

## Extension Configuration

- api: API allows specifying `api::key` and `api::site`. This is only effective when the extension is used in the OSS collector built via OCB. In this case, the profiles are sent directly to the intake using the API key and site specified.
- profiler_options: Profiler options allows configuring options that pertain to the profiler, such as service/env/version, or period (in seconds).
- endpoint: In the OTel agent, the extension spins up an http server which receives the profiles and sends them to DD intake. You can change the endpoint at which the server listens via this config (default port `7501`)


Example Config OSS collector:
```
extensions:
  ddprofiling: 
    api:
      key: ${env:DD_API_KEY}
      site: ${env:DD_SITE}
```

Example Config OTel Agent:
```
extensions:
  ddprofiling: 
    endpoint: 1234
```

Example profiler options config:
```
extensions:
  ddprofiling:
    profiler_options:
      service: svc
      version: v0.1
      env: env
      period: 30
      profile_types: [blockprofile, mutexprofile, goroutineprofile]
```
