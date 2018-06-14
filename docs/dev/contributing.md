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

To install reno: `pip install reno`

We use `Reno` to create our CHANGELOG. Reno is a pretty simple
[tool](https://docs.openstack.org/reno/latest/user/usage.html).

Ultra quick `Reno` HOWTO:

```bash
$> reno new <topic-of-my-pr> --edit
[...]
# Remove unused section and fill the relevant ones
# Reno will create a new file in releasenotes/notes
[...]
```

Then just add and commit the new releasenote (located in `releasenotes/notes/`) with your PR.

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
