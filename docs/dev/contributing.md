# Contributing to Datadog Agent

First of all, thanks for contributing!

This document provides some basic guidelines for contributing to this repository.
To propose improvements, feel free to submit a PR.

## Submitting issues

  * If you think you've found an issue, please search the [Troubleshooting][troubleshooting]
    section of our [Knowledge base][kb] to see if it's known.
  * If you can't find anything useful, please contact our [support][support] and
    [send them your logs][flare].
  * Finally, you can open a Github issue.

## Pull Requests

Have you fixed a bug or written a new check and want to share it? Many thanks!

In order to ease/speed up our review, here are some items you can check/improve
when submitting your PR:

  * have a [proper commit history](#commits) (we advise you to rebase if needed).
  * write tests for the code you wrote.
  * preferably make sure that all tests pass locally.
  * summarize your PR with an explanatory title and a message describing your
    changes, cross-referencing any related bugs/PRs.
  * use [Reno](#reno) to create a releasenote.
  * open your PR against the `master` branch.
  * set the `team/agent-core` label
  * add a milestone to your PR (use the highest available, ex: `6.8.0`)

Your pull request must pass all CI tests before we will merge it. If you're seeing
an error and don't think it's your fault, it may not be! [Join us on Slack][slack]
or send  us an email, and together we'll get it sorted out.

### Keep it small, focused

Avoid changing too many things at once. For instance if you're fixing the NTP
check and at the same time shipping a dogstatsd improvement, it makes reviewing
harder and the _time-to-release_ longer.

### Commit Messages

Please don't be this person: `git commit -m "Fixed stuff"`. Take a moment to
write meaningful commit messages.

The commit message should describe the reason for the change and give extra details
that will allow someone later on to understand in 5 seconds the thing you've been
working on for a day.

If your commit is only shipping documentation changes or example files, and is a
complete no-op for the test suite, please add **[skip ci]** in the commit message
body to skip the build and give that slot to someone else who does need it.

### Squash your commits

Please rebase your changes on `master` and squash your commits whenever possible,
it keeps history cleaner and it's easier to revert things. It also makes developers
happier!

### Reno

We use `Reno` to create our CHANGELOG. Reno is a pretty simple
[tool](https://docs.openstack.org/reno/latest/user/usage.html). With each PR
should come a new releasenotes created with `reno` (unless your change doesn't
have a single user impact and should not be mentioned in the CHANGELOG, very
unlikely !).

To install reno: `pip install reno`

Ultra quick `Reno` HOWTO:

```bash
$> reno new <topic-of-my-pr> --edit
[...]
# Remove unused sections and fill the relevant ones.
# Reno will create a new file in releasenotes/notes.
#
# Each section from every releasenote are combined when the CHANGELOG.rst is
# rendered. So the text needs to be worded so that it does not depend on any
# information only available in another section. This may mean repeating some
# details, but each section must be readable independently of the other.
#
# Each section note must be formatted as reStructuredText.
[...]
```

Then just add and commit the new releasenote (located in `releasenotes/notes/`)
with your PR. If the change is on the `trace-agent` (folders `cmd/trace-agent` or `pkg/trace`)
please prefix the release note with "APM :" and the <topic-of-my-pr> argument with
"apm-".

#### Reno sections

The main thing to keep in mind is that the CHANGELOG is written for the agent's
users and not its developers.

- `features`: describe shortly what your feature does.

  example:
  ```yaml
  features:
    - |
      Introducing the Datadog Process Agent for Windows.
  ```

- `enhancements`: describe enhancements here: new behavior that are too small
  to be considered a new feature.

  example:
  ```yaml
  enhancements:
    - |
      Windows: Add PDH data to flare.
  ```

- `issues`: describe known issues or limitation of the agent.

  example:
  ```yaml
  issues:
    - |
      Kubernetes 1.3 & OpenShift 3.3 are currently not fully supported: docker
      and kubelet integrations work OK, but apiserver communication (event
      collection, `kube_service` tagging) is not implemented
  ```

- `upgrade`: List actions to take or limitations that could arise upon upgrading the agent. Notes here must include steps that users can follow to 1. know if they're affected and 2. handle the change gracefully on their end.

  example:
  ```yaml
  upgrade:
    - |
      If you run a Nomad agent older than 0.6.0, the `nomad_group`
      tag will be absent until you upgrade your orchestrator.
  ```

- `deprecations`: List deprecation notes here.

  example:
  ```yaml
  deprecations:
  - |
    Changed the attribute name to enable log collection from YAML configuration
    file from "log_enabled" to "logs_enabled", "log_enabled" is still
    supported.
  ```

- `security`: List security fixes, issues, warning or related topics here.

  example:
  ```yaml
  security:
    - |
      The /agent/check-config endpoint has been patched to enforce
      authentication of the caller via a bearer session token.
  ```

- `fixes`: List the fixes done in your PR here. Remember to be clear and give a
  minimum of context so people reading the CHANGELOG understand what the fix is
  about.

  example:
  ```yaml
  fixes:
    - |
      Fix EC2 tags collection when multiple marketplaces are set.
  ```

- `other`: Add here every other information you want in the CHANGELOG that
  don't feat in any other section. This section should rarely be used.

  example:
  ```yaml
  other:
    - |
      Only enable the ``resources`` metadata collector on Linux by default, to match
      Agent 5's behavior.
  ```

## Integrations

Also called checks, all officially supported Agent integrations live in the
[integrations-core][core] repo. Please look there to submit related issues, PRs,
or review the latest changes. For new integrations, please open a pull request
in the [integrations-extras][extras] repo.


[troubleshooting]: https://datadog.zendesk.com/hc/en-us/sections/200766955-Troubleshooting
[kb]: https://datadog.zendesk.com/hc/en-us
[support]: http://docs.datadoghq.com/help/
[flare]: https://github.com/DataDog/dd-agent/wiki/Send-logs-to-support
[extras]: https://github.com/DataDog/integrations-extras
[core]: https://github.com/DataDog/integrations-core
[slack]: http://datadoghq.slack.com
