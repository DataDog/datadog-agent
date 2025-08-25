---
title: Creating Agent Rule Expressions
description: "Agent expression attributes and operators for Workload Protection Rules"
disable_edit: true
further_reading:
- link: "/security/cloud_workload_security/getting_started/"
  tag: "Documentation"
  text: "Get started with Datadog Workload Protection"
---

## Create custom rules using the Assisted rule creator

The **Assisted rule creator** option helps you create the Agent and dependent detection rules together, and ensures that the Agent rule is referenced in the detection rules. Using this tool is faster than the advanced method of creating the Agent and detection rules separately.

For details, see [Creating Custom Detection Rules][1].

## Agent expression syntax
Workload Protection first evaluates activity within the Datadog Agent against Agent expressions to decide what activity to collect. This portion of a Workload Protection rule is called the Agent expression. Agent expressions use Datadog's Security Language (SECL). The standard format of a SECL expression is as follows:

{{< code-block lang="javascript" >}}
<event-type>.<event-attribute> <operator> <value> [<operator> <event-type>.<event-attribute>] ...

{{< /code-block >}}

Using this format, an example rule for a Linux system looks like this:

{{< code-block lang="javascript" >}}
open.file.path == "/etc/shadow" && process.file.path not in ["/usr/sbin/vipw"]

{{< /code-block >}}

## Operators
SECL operators are used to combine event attributes together into a full expression. The following operators are available:

| SECL Operator         |  Definition                              | Agent Version |
|-----------------------|------------------------------------------|---------------|
| `==`                  | Equal                                    | 7.27          |
| `!=`                  | Not equal                                | 7.27          |
| `>`                   | Greater                                  | 7.27          |
| `>=`                  | Greater or equal                         | 7.27          |
| `<`                   | Lesser                                   | 7.27          |
| `<=`                  | Lesser or equal                          | 7.27          |
| `!` or `not`          | Not                                      | 7.27          |
| `^`                   | Binary not                               | 7.27          |
| `in [elem1, ...]`     | Element is contained in list             | 7.27          |
| `not in [elem1, ...]` | Element is not contained in list         | 7.27          |
| `=~`                  | String matching                          | 7.27          |
| `!~`                  | String not matching                      | 7.27          |
| `&`                   | Binary and                               | 7.27          |
| `\|`                  | Binary or                                | 7.27          |
| `&&` or `and`         | Logical and                              | 7.27          |
| `\|\|` or `or`        | Logical or                               | 7.27          |
| `in CIDR`             | Element is in the IP range               | 7.37          |
| `not in CIDR`         | Element is not in the IP range           | 7.37          |
| `allin CIDR`          | All the elements are in the IP range     | 7.37          |
| `in [CIDR1, ...]`     | Element is in the IP ranges              | 7.37          |
| `not in [CIDR1, ...]` | Element is not in the IP ranges          | 7.37          |
| `allin [CIDR1, ...]`  | All the elements are in the IP ranges    | 7.37          |

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

{{< code-block lang="javascript" >}}
open.file.path == "/etc/secret" && process.file.name == "java" && process.created_at > 5s

{{< /code-block >}}

Durations are numbers with a unit suffix. The supported suffixes are "s", "m", "h".

## Platform specific syntax

SECL expressions support several platforms. You can use the documentation below to see what attributes and helpers are available for each.

* [Linux][2]
* [Windows][3]

[1]: /security/threats/workload_security_rules/custom_rules
[2]: /security/threats/linux_expressions
[3]: /security/threats/windows_expressions