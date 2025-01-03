### Logging

Logging utilizes the [`github.com/cihub/seelog`](https://github.com/cihub/seelog) package as its underlying framework. 
It is accessible via `pkg/util/log` and the `comp/core/log` component wrapper.
Using the component wrapper is recommended, as it adheres to [component best practices](https://datadoghq.dev/datadog-agent/components/overview/).

#### Writing a good log message

In general, there are both a few rules and a few suggestions to follow when it comes to writing a
(good) log message:

- Messages must be written in English. No preference on which specific English dialect is used e.g.
  American English, British English, Canadian English, etc.
- Sentences must be capitalized, and end with a period.
- Proper spelling and grammar when possible. Not all of us are native English speakers, and so this
  is simply an ask, but not a hard requirement.
- Identifiers, or passages of note, should be called out by some means i.e. wrapping them in
  backticks or quotes.  Wrapping with special characters can be helpful in drawing the users eye to
  anything of importance.
- If it's longer than one or two sentences, it's probably better suited as a single sentence briefly
  explaining the event, with a link to external documentation that explains further.

#### Choosing the right log level

Similarly, choosing the right level can be important, both from the perspective of making it easy
for users to grok what they should pay attention to, but also to avoid the performance overhead of
excess logging (even if we filter it out and it never makes it to the console).

- **TRACE**: Typically contains a high level of detail for deep/rich debugging.

  As trace logging is typically reached for when instrumenting algorithms and core pieces of logic,
  care should be taken to avoid trace logging being added to tight loops, or commonly used
  codepaths, where possible. Even when disabled, there can still be a small overhead associated with
  logging an event at all.
- **DEBUG**: Basic information that can be helpful for initially debugging issues.

  Should typically not be used for things that happen per-event, or scales with event throughput,
  but in some cases -- i.e. if it happens every 1000th event, etc -- it can safely be used.
- **INFO**: Common information about normal processes.

  This includes logical/temporal events such as notifications when components are stopped and
  started, or other high-level events that, crucially, do not represent an event that an operator
  needs to worry about.

  Said another way, **INFO** is primarily there for information that lets them know that an action
  they just took completed successfully.
- **WARN**: Something unexpected happened, but no data has been lost, nothing has crashed, and we
  can recover from it without an issue. An operator might be interested in something at the **WARN**
  level, but it shouldn't be informing them of things serious enough to require immediate attention.
- **ERROR**: Data loss, unrecoverable errors, and anything else that will require an operator to
  intervene and recover from. These should be rare so that they maintain a high signal-to-noise
  ratio in the observability tooling that operators themselves are using.
