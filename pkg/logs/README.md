# logs-agent

The logs-agent collects logs and submits them to DataDog's infrastructure.

## Agent Structure

There are two major "parts" of the logs agent: what to log, and the mechanics of logging.

### What to Log (Schedulers and Launchers)

The first part has an architecture like this:

```
                                  Autodiscovery
                                        │
                                        │integration.Config
                                        │
             ┌ -Schedulers - - - - - - -▼- - - - - - ┐
               ┌──────────────┐   ┌──────────────┐
             | │   Scheduler  │   │ ad.Scheduler │ … |
               └──────────────┘   └──────────────┘
             └ - - - - - - - - - - - - - - - - - - - ┘
              config.LogSource│    │service.Service
                              │    │
                              ▼    ▼
                     ┌─────────┐  ┌──────────┐
            ┌────────┤ Sources │  │ Services │
            ▼        └┬──┬─────┘  └─┬──────┬─┘
      ┌───────────┐   │  │   ▲ ▲    │      │
   ┌──┤ Launchers │   │  │   │ │    │      │
   │  └───────────┘   │  │ ┌────────┘      │
   │                  ▼  │ │ │ │           │
   │  ┌────────────────┐ │ │ │ │           │
   ├──┤ file.Launcher  │ │ │ │ │           │
   │  └────────────────┘ │ │ │ │           │
   │                     ▼ ▼ │ │           ▼
   │  ┌────────────────────┐ │ │ ┌─────────────────────┐
   ├──┤ docker.Launcher    ├─┘ └─┤ kubernetes.Launcher │
   │  └────────────────────┘     └─────────────────────┘
   │    ▲                          ▲
   │    └──container labels        └──pod annotations
   ▼
tailers
```

#### Scheduling

The logs agent maintains a collection of *schedulers*, which are responsible for managing logs sources and logs services.
Schedulers add and remove sources and services dynamically during agent runtime.

A *Source* is an integration with LogsConfig that describes a source of log messages to be handled.
A *Service* is a container, used to support `container_collect_all`.

Sources and services go into separate stores of active sources and active services.
The remaining components of the logs agent subscribe to these stores and take appropriate action.

Schedulers can be implemented outside of the logs-agent, but some built-in schedulers are in sub-packages of `pkg/logs/schedulers`.
One particularly important scheduler is the *AD scheduler* in `pkg/logs/schedulers/ad`.
The Autodiscovery component (`pkg/autodiscovery`) provides a sequence of configs (`integration.Config`) to the AD scheduler.
The AD scheduler categorizes each config as either a source or a service and submits it accordingly.

#### Launchers

Launchers are implemented in sub-packages of `pkg/internal/launchers`.
They are responsible for creating tailers, which contain pipelines -- see below.
Each filters the sources, usually based on `source.Config.Type`, which comes from the `type` key in `logs_config` sections.

Several Launchers are quite simple, translating sources into a tailers.
For example:

* The listener launcher creates a new tailer for each configured UDP port, or for each incoming connection on a configured TCP port.

The launchers depicted separately in the diagram above have some additional behaviors.

##### file.Launcher

The file launcher handles wild-card filenames and logfile rotation by creating multiple tailers for each source.

##### docker.Launcher

The Docker launcher consumes both sources and services from the scheduler.
It reconciles these two streams to find matching sources and services, and only when both have arrived does it start a new tailer.

When `logs_config.container_collect_all` is enabled, the launcher also starts a tailer for any docker container that does not have a corresponding source and which does not have autodiscovery-related labels.
This functionality has some inherent race conditions.
The logs-agent delays startup of the `container_collect_all` support until after autodiscovery has scanned its configuration sources once, so that all file-based source definitions are already in place when it begins handling services.

The agent can use two mechanisms to capture log messages from a Docker container (configured with `logs_config.docker_container_use_file` and `logs_config.docker_container_force_use_file`):
 * Docker API - the launcher creates a tailer which reads from the Docker API socket and sends messages into the logging pipeline.
 * File - the launcher determines the on-disk filename of the container's logfile and creates a "child" `config.LogSource`  with `source.Config.Type = "file"`.
   The file launcher receives this source from the sources store and tails the logfile.

##### kubernetes.Launcher

The Kubernetes launcher never produces tailers, and only subscribes to services.
For each service (container) it finds, it determines which pod the container represents, and consults the annotations on that pod.
It then generates a `"file"` source, similar to that produced by the Docker launcher, and adds it to the sources store.

### Logging Messages (Tailers and Pipelines)

The second portion of the logs agent looks like this:

```
  ┌───────────────────┐
┌►│      Tailer       │  ─┐
│ └─────────┬─────────┘   │
│           │             │ input
│           ▼             │ (many)
│ ┌───────────────────┐   │
│ │      Decoder      │  ─┘
│ └─────────┬─────────┘
│           │
│           ▼
│ ┌───────────────────┐
│ │     Processor     │  ─┐
│ └─────────┬─────────┘   │
│           │             │
│           ▼             │
│ ┌───────────────────┐   │
│ │     Strategy      │   │
│ └─────────┬─────────┘   │
│           │             │ pipeline
│           ▼             │ (few)
│ ┌───────────────────┐   │
│ │      Sender       │   │
│ └─────────┬─────────┘   │
│           │             │
│           ▼             │
│ ┌───────────────────┐   │
│ │    Destination    │  ─┘
│ └─────────┬──────┬──┘
│           │      └────►DataDog
│           ▼            Intake
│ ┌───────────────────┐
│ │      Auditor      │
│ └─────────┬─────────┘
└───────────┘
```

### Inputs

Each input is composed of a tailer and (sometimes) a decoder, as created by the portion described above.
One such input exists for each source of logs -- potentially many in a single agent.

A decoder translates a sequence of byte buffers (such as from a file or a network socket) into log messages.
Decoders and their components are defined in `pkg/logs/decoder`.

In many cases, an input consists only of a tailer, so inputs are sometimes referred to as tailers.

Tailers are defined in sub-packages of `pkg/logs/internal/tailers`.

### Pipelines

Each input writes to a pipeline, consisting of a processor, strategy, sender, and destinations.
There are a limited number of pipelines for an agent, to take advantage of CPU parallelism, and inputs are distributed evenly among those pipelines.
Each pipeline handles messages in-order, so assigning each input to a single pipeline ensures that input's messages will be delivered to the intake in-order.

The components of a pipeline are:

* `Processor` updates the messages, filtering, redacting, or adding metadata, and submits to the strategy.
* `Strategy` converts a stream of messages into a stream of encoded payloads, batching if needed.
* `Sender` submits the payloads to the destination(s).
* `Destination` sends the actual encoded content to the intake, retries if needed, and sends successful payloads to the auditor.

### Auditor

Finally, pipelines report successful message transmission to the auditor, which informs tailers.
Some inputs use this information to track messages that have been delivered successfully, allowing them to continue re-transmitting unsuccessful messages after agent restart.
