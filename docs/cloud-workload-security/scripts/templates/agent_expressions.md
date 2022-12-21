---
title: Creating Custom Agent Rules
kind: documentation
description: "Agent expression attributes and operators for Cloud Workload Security Rules"
disable_edit: true
further_reading:
- link: "/security/cloud_workload_security/getting_started/"
  tag: "Documentation"
  text: "Get started with Datadog Cloud Workload Security"
---

{{ warning_message }}

## Agent expression syntax
Cloud Workload Security (CWS) first evaluates activity within the Datadog Agent against Agent expressions to decide what activity to collect. This portion of a CWS rule is called the Agent expression. Agent expressions use Datadog's Security Language (SECL). The standard format of a SECL expression is as follows:

{% raw %}
{{< code-block lang="javascript" >}}
{% endraw %}
<event-type>.<event-attribute> <operator> <value> <event-attribute> ...
{% raw %}
{{< /code-block >}}
{% endraw %}

Using this format, an example rule looks like this:
{% raw %}
{{< code-block lang="javascript" >}}
{% endraw %}
open.file.path == "/etc/shadow" && file.path not in ["/usr/sbin/vipw"]
{% raw %}
{{< /code-block >}}
{% endraw %}

## Triggers
Triggers are events that correspond to types of activity seen by the system. The currently supported set of triggers is:

| SECL Event | Type | Definition | Agent Version |
| ---------- | ---- | ---------- | ------------- |
{% for event_type in event_types %}
{% if event_type.name != "*" %}
| `{{ event_type.name }}` | {{ event_type.kind }} | {{ "[Experimental] " if event_type.experimental else "" }}{{ event_type.definition }} | {{ event_type.min_agent_version }} |
{% endif %}
{% endfor %}

## Operators
SECL operators are used to combine event attributes together into a full expression. The following operators are available:

| SECL Operator         | Types            |  Definition                              | Agent Version |
|-----------------------|------------------|------------------------------------------|---------------|
| `==`                  | Process          | Equal                                    | 7.27          |
| `!=`                  | File             | Not equal                                | 7.27          |
| `>`                   | File             | Greater                                  | 7.27          |
| `>=`                  | File             | Greater or equal                         | 7.27          |
| `<`                   | File             | Lesser                                   | 7.27          |
| `<=`                  | File             | Lesser or equal                          | 7.27          |
| `!`                   | File             | Not                                      | 7.27          |
| `^`                   | File             | Binary not                               | 7.27          |
| `in [elem1, ...]`     | File             | Element is contained in list             | 7.27          |
| `not in [elem1, ...]` | File             | Element is not contained in list         | 7.27          |
| `=~`                  | File             | String matching                          | 7.27          |
| `!~`                  | File             | String not matching                      | 7.27          |
| `&`                   | File             | Binary and                               | 7.27          |
| `\|`                  | File             | Binary or                                | 7.27          |
| `&&`                  | File             | Logical and                              | 7.27          |
| `\|\|`                | File             | Logical or                               | 7.27          |
| `in CIDR`             | Network          | Element is in the IP range               | 7.37          |
| `not in CIDR`         | Network          | Element is not in the IP range           | 7.37          |
| `allin CIDR`          | Network          | All the elements are in the IP range     | 7.37          |
| `in [CIDR1, ...]`     | Network          | Element is in the IP ranges              | 7.37          |
| `not in [CIDR1, ...]` | Network          | Element is not in the IP ranges          | 7.37          |
| `allin [CIDR1, ...]`  | Network          | All the elements are in the IP ranges    | 7.37          |

## Patterns and regular expressions
Patterns or regular expressions can be used in SECL expressions. They can be used with the `in`, `not in`, `=~`, and `!~` operators.

| Format           |  Example             | Supported Fields   | Agent Version |
|------------------|----------------------|--------------------|---------------|
| `~"pattern"`     | `~"httpd.*"`         | All                | 7.27          |
| `r"regexp"`      | `r"rc[0-9]+"`        | All except `.path` | 7.27          |

Patterns on `.path` fields will be used as Glob. `*` will match files and folders at the same level. `**`, introduced in 7.34, can be used at the end of a path in order to match all the files and subfolders.

## Duration
You can use SECL to write rules based on durations, which trigger on events that occur during a specific time period. For example, trigger on an event where a secret file is accessed more than a certain length of time after a process is created.
Such a rule could be written as follows:

{% raw %}
{{< code-block lang="javascript" >}}
open.file.path == "/etc/secret" && process.file.name == "java" && process.created_at > 5s

{{< /code-block >}}
{% endraw %}

Durations are numbers with a unit suffix. The supported suffixes are "s", "m", "h".

## Variables
SECL variables are predefined variables that can be used as values or as part of values.

For example, rule using a `process.pid` variable looks like this:

{% raw %}
{{< code-block lang="javascript" >}}
open.file.path == "/proc/${process.pid}/maps"

{{< /code-block >}}
{% endraw %}

List of the available variables:

| SECL Variable         |  Definition                           | Agent Version |
|-----------------------|---------------------------------------|---------------|
| `process.pid`         | Process PID                           | 7.33          |

## CIDR and IP range
CIDR and IP matching is possible in SECL. One can use operators such as `in`, `not in`, or `allin` combined with CIDR or IP notations.

Such rules can be written as follows:

{% raw %}
{{< code-block lang="javascript" >}}
dns.question.name == "example.com" && network.destination.ip in ["192.168.1.25", "10.0.0.0/24"]

{{< /code-block >}}
{% endraw %}

## Helpers
Helpers exist in SECL that enable users to write advanced rules without needing to rely on generic techniques such as regex.

### Command line arguments
The *args_flags* and *args_options* are helpers to ease the writing of CWS rules based on command line arguments.

*args_flags* is used to catch arguments that start with either one or two hyphen characters but do not accept any associated value.

Examples:
* `version` is part of *args_flags* for the command `cat --version`
* `l` and `n` both are in *args_flags* for the command `netstat -ln`


*args_options* is used to catch arguments that start with either one or two hyphen characters and accepts a value either specified as the same argument but separated by the ‘=’ character or specified as the next argument.

Examples:
* `T=8` and `width=8` both are in *args_options* for the command `ls -T 8 --width=8`
* `exec.args_options ~= [ “s=.*\’” ]` can be used to detect `sudoedit` was launched with `-s` argument and a command that ends with a `\`

### File rights

The *file.rights* attribute can now be used in addition to *file.mode*. *file.mode* can hold values set by the kernel, while the *file.rights* only holds the values set by the user. These rights may be more familiar because they are in the `chmod` commands.

## Event types

{% for event_type in event_types %}
{% if event_type.name == "*" %}
### Common to all event types
{% else %}
### Event `{{ event_type.name }}`

{% if event_type.experimental %}
_This event type is experimental and may change in the future._

{% endif %}
{{ event_type.definition }}
{% endif %}

| Property | Type | Definition | Constants |
| -------- | ---- | ---------- | --------- |
{% for property in event_type.properties %}
| `{{ property.name }}` | {{ property.datatype }} | {{ property.definition }} | {{ property.constants }} |
{% endfor %}

{% endfor %}

## Constants

Constants are used to improve the readability of your rules. Some constants are common to all architectures, others are specific to some architectures.

{% for constants in constants_list %}
### `{{ constants.name }}`

{{ constants.definition }}

| Name | Architectures |
| ---- |---------------|
{% for constant in constants.all %}
| `{{ constant.name }}` | {{ constant.architecture }} |
{% endfor %}

{% endfor %}

{% raw %}
{{< partial name="whats-next/whats-next.html" >}}
{% endraw %}
