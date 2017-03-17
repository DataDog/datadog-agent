# Contributing to Datadog Agent

First of all, thanks for contributing!

This document aims to provide some basic guidelines to contribute to this repository, but keep in mind that these are just guidelines, not rules; use your best judgment and feel free to propose changes to this document in a pull request.

## Submitting issues

- You can first take a look at the [Troubleshooting](https://datadog.zendesk.com/hc/en-us/sections/200766955-Troubleshooting) section of our [Knowledge base](https://datadog.zendesk.com/hc/en-us).
- If you can't find anything useful, please contact our [support](http://docs.datadoghq.com/help/) and [send them your logs](https://github.com/DataDog/dd-agent/wiki/Send-logs-to-support).
- Finally, you can open a Github issue respecting this [convention](#commits-titles) (it helps us triage).


## Pull Requests

You wrote some code/added a new check and want to share it? Thanks a lot for your interest!

In order to ease/speed up our review, here are some items you can check/improve when submitting your PR:

- [ ] have a [proper commit history](#commits) (we advise you to rebase if needed).
- [ ] write tests for the code you wrote.
- [ ] preferably make sure that all tests pass locally.
- [ ] summarize your PR with a [good title](#commits-titles) and a message describing your changes, cross-referencing any related bugs/PRs.

Your Pull Request **must** always pass all the CI tests before being merged, if you think the error is not due to your changes, you can have a talk with us on Slack (datadoghq.slack.com) or send us an email to support _at_ datadoghq _dot_ com)

_If you are adding a dependency (python module, library, ...), please check the [corresponding section](#add-dependencies)._

## [Integrations](https://github.com/DataDog/integrations-core)

Most of the checks live in the [Integration SDK](https://github.com/DataDog/integrations-core). Please look there to submit related issues, PRs, or review the latest changes.

For new integrations, please open a pull request in the [integrations extras repo](https://github.com/DataDog/integrations-extras)

## Commits

### Keep it small, focused

Avoid changing too many things at once, for instance if you're fixing the redis integration and at the same time shipping a dogstatsd improvement, it makes reviewing harder (devs specialize in different parts of the code) and the change _time-to-release_ longer.

### Bisectability

Every commit should lead to a valid code, at least a code in a better state than before. That means that every revision should be able to pass unit and integration tests ([more about testing](#tests))

An **example** of something which breaks bisectability:
* commit 1: _Added check X_
* commit 2: _forgot column_
* commit 3: _fix typo_

To avoid that, please rebase your changes and create valid commits. It keeps history cleaner, it's easier to revert things, and it makes developers happier too.


### Messages

Please don't use `git commit -m "Fixed stuff"`, it usually means that you just wrote the very first thing that came to your mind without much thought. Also it makes navigating through the code history harder.

Instead, the commit shortlog should focus on describing the change in a sane way (see [commits titles](#commits-titles)) and be **short** (72 columns is best).

The commit message should describe the reason for the change and give extra details that will allow someone later on to understand in 5 seconds the thing you've been working on for a day.

If your commit is only shipping documentation changes or example files, and is a complete no-op for the test suite, please add **[skip ci]** in the commit message body to skip the build and let you build slot to someone else _in need :wink:_

Examples, see:
  * https://github.com/DataDog/dd-agent/commit/44bc927aaaf2925ef081768b5888bbb20a5bb3bd
  * https://github.com/DataDog/dd-agent/commit/677417fe12b1914e4322ac2c1fd1645cb0f1de31
  * and for more general guidance, [this should help](http://chris.beams.io/posts/git-commit/)

### Commits titles

Every commit title, PR or issue should be named like the following example:
```
[category] short description of the matter
```

`category` can be:
* _core_: for the agent internals, or the common interfaces
* _dogstatsd_: for the embedded dogstatsd server
* _tests_: related to CI, integration & unit testing
* _dev_: related to development or tooling
* _check_name_: specific to one check

For descriptions, keep it short keep it meaningful. Here are a few examples to illustrate.

#### Bad descriptions

* [mysql] mysql check does not work
* [snmp] improved snmp
* [core] refactored stuff

#### Good descriptions

* [mysql] exception ValueError on mysql 5.4
* [snmp] added timeouts to snmpGet calls
* [core] add config option to common metric interface

## Add dependencies

You wrote a new agent check which uses a dependency not embedded in the agent yet? You're at the right place to correct this!

We use [Omnibus](https://github.com/chef/omnibus) to build our agent and bundle all dependencies and we define what are the agent
dependencies in the `omnibus` folder.
