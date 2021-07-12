## Supported architectures

[`amd64`](https://hub.docker.com/r/datadog/agent-amd64) , [`arm64v8`](https://hub.docker.com/r/datadog/agent-arm64)

## Supported versions

#### Agent 7

The Datadog agent, including a Python 3 interpreter for Python checks.

Relevant tags are:

- `7` , `7-jmx` : use these if you want to track the latest `7` minor release, without breaking change.
- `7.X.X` , `7.X.X-jmx` : use these if you want to pin the agent to a precise version (don't forget to upgrade regularly for the latest features).
- `latest` , `latest-jmx` : use these for following the latest agent release, but keep in mind that it will update automatically to the next major release, which will have breaking changes. Major releases are very infrequent (less than one per year).

#### Agent 6

The Datadog agent, including a Python 2 interpreter for Python checks. Note that Python 2 EOL is set for January 1, 2020.

Relevant tags are:

- `6` , `6-jmx`
- `6.X.X` , `6.X.X-jmx`
- `latest-py2` , `latest-py2-jmx`

## Image variants

The agent image comes in two flavors, each one fulfilling a specific use case.

### agent:\<version\>-jmx

This variant embeds a Java Runtime Environment for JMX-based checks. If you
are uncertain about what your needs are, this is probably the one you should
use.

### agent:\<version\>

This variant doesn't embed a Java runtime. If you don't plan on using
JMX-based checks, you probably want to pick this one since it is noticeably
slimmer.

## Documentation

Please refer to:

- [https://docs.datadoghq.com/](https://docs.datadoghq.com/)
- [usage instructions](https://github.com/DataDog/datadog-agent/tree/main/Dockerfiles/agent) for this image
- [general agent documentation](https://github.com/DataDog/datadog-agent/tree/main/docs) in our repo

## Support

For issues and help troubleshooting, please [contact our support team](https://www.datadoghq.com/support/). If you want to contribute, or think you found a bug in the agent, [let's talk on our github repository](https://github.com/DataDog/datadog-agent).

## License

View [license information](https://github.com/DataDog/datadog-agent/blob/main/LICENSE) for the software contained in this image.

As with all Docker images, these likely also contain other software which may be under other licenses (such as Bash, etc from the base distribution, along with any direct or indirect dependencies of the primary software being contained).

As for any pre-built image usage, it is the image user's responsibility to ensure that any use of this image complies with any relevant licenses for all software contained within.
