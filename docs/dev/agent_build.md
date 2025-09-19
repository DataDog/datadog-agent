# Building the Agent

## Testing Agent changes in containerized environments

Building an Agent Docker image from scratch through an embedded build is a slow process.
You can quickly test a change or bug fix in a containerized environment (such as Docker, Kubernetes, or ECS).

One way to do this is to patch the Agent binary from an official Docker image, with a Dockerfile:

```
FROM datadog/agent:<AGENT_VERSION>

COPY agent /opt/datadog-agent/bin/agent/agent
```

For this to work properly, two things are important:
- Your change needs to be done on top of the `<AGENT_VERSION>` tag from the DataDog repository.
- You need to run the invoke task with the proper embedded path `dda inv -e agent.build -e /opt/datadog-agent/embedded`.

**Note**: This makes `invoke` install the build's artifacts in the `/opt/datadog-agent/embedded` folder. Make sure the folder exists and the current user has write permissions.
