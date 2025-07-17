### Logging

Logging utilizes the [`github.com/cihub/seelog`](https://github.com/cihub/seelog) package as its underlying framework. 
You can access logging through `pkg/util/log` and the `comp/core/log` component wrappers.
Using the component wrapper is recommended, as it adheres to [component best practices](https://datadoghq.dev/datadog-agent/components/overview/).

#### Writing a good log message

In general, there are a few rules and a few suggestions to follow when it comes to writing
good log messages:

- Messages must be written in English, preferably in American English.
- Use proper spelling and grammar when possible. Because not everyone is a native English speaker, this is an ask, not a hard requirement.
- Identifiers or passages of note should be called out by some means such as wrapping them in
  backticks or quotes (single or double). Wrapping with special characters can be helpful in drawing the user's eye to anything of importance.
- If the message is longer than one or two sentences, it's probably better suited as a single sentence briefly
  explaining the event, with a link to external documentation that explains things further.

#### Choosing the right log level

Choosing the right level is also very important. Appropriate log levels make it easy for users to understand what they should pay attention to. 
They also avoid the performance overhead of excess logging, even if the logs are filtered and never show on the console.

- **TRACE**: Typically contains a high level of detail for deep/rich debugging.

  Trace logging is typically used when instrumenting algorithms and core pieces of logic. 
  Avoid adding trace logging to tight loops or commonly used codepaths.
  Even when the logs are disabled, logging an event can incur overheads.
- **DEBUG**: Basic information that can be helpful for initially debugging issues.

  Do not use debug logging for things that happen per-event or that scale with event throughput.
  You can safely use debug logging for uncommon cases, for example, something that happens every 1000th event.
- **INFO**: Common information about normal processes.

  Info logging is appropriate for logical or temporal events.
  Examples include notifications when components are stopped and started, or other high-level events that do not require operator attention.

  **INFO** is primarily used for information that tells an operator that a notable action completed successfully.

- **WARN** should be used for potentially problematic but non-critical events where the software can continue operating, 
  potentially in a degraded state and/or recover from the problem. Do not use **WARN** for events that require user's immediate attention.
  
- **ERROR** level should be used for events indicating severely problematic issues that require immediate user visibility and remediation.
  This includes logging related to events that may lead to data loss, unrecoverable states, and any other situation where a required component is faulty,
  causing the software to be unable to remediate the problem on its own.
  Error logs should be extremely rare in normally operating software to ensure high signal-to-noise ratio in observability tooling.
