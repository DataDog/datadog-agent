# Datadog Agent Testing

This directory contains automated integration tests for the Datadog Agent. It
currently tests Agent packaging (installing, updating, and removing the Agent),
as well as basic functionality. The tests are executed on virtual machines that
span most of the supported Agent platforms. They're currently run on Azure VMs.

This is meant to be executed internally at Datadog and references our internal build structure.
However, people outside of Datadog can run it by editing the repository URL that they are uploaded to in the .kitchen-azure.yml file. You will also need to ensure that the repo branch is `pipeline-$CI_PIPELINE_ID`.

## Getting Started

### Dependencies
Non-bundled dependencies:
 - [Ruby](http://www.ruby-lang.org/) (last tested with 2.2.1)
 - [Bundler](http://bundler.io/)

Then install bundled gem dependencies:
 ` bundle install `

Note: you might run into an error building the `nio4r` native extensions. You
should be able to get around that by setting the build cflags for the gem
in bundler:
 ` bundle config build.nio4r --with-cflags="-std=c99" `

#### Azure

These tests are set up to be run on Azure.
It is set up to use our automated testing.
However, there are also provisions to run it locally.

You will need to create a service principle.
You can do that with the following command on the azure cli tool:

```
az ad sp create-for-rbac --name <name>
```

Then create a file called `azureid.sh` and export the relevant settings into environment variables:

The subscription ID is retrieved from the azure portal, by starting a azure shell. --or-- the "id" returned from `az account list`

The AZURE_CLIENT_ID is returned as the appId from the above command

the AZURE_CLIENT_SECRET is returned as the password from the above command

the AZURE_TENANT_ID is returned as tenant from the above command --or-- the "tenantId" returned from `az account list`

```
export AZURE_CLIENT_ID="$AZURE_CLIENT_ID"
export AZURE_CLIENT_SECRET="$AZURE_CLIENT_SECRET"
export AZURE_TENANT_ID="$AZURE_TENANT_ID"
export AZURE_SUBSCRIPTION_ID="$AZURE_SUBSCRIPTION_ID"
export CI_PIPELINE_ID="$CI_PIPELINE_ID"
export NOT_GITLAB="true"
```

##### Images

If for some reason you need to find another version of a specific image you will
need to use the Azure CLI.

This will list all the images available on Azure (and take ~10min to run)
```bash
az vm image list --all --output table
```

This will list all the images available on Azure for a specific OS (and take ~2min to run)
```bash
az vm image list --offer Ubuntu --all --output table
```

#### Common

To see the rest of the rake commands, execute

    rake -T

For finer control, you can invoke test-kitchen directly

To list all available tests, execute

    kitchen list

And to run one of these tests, execute

    kitchen test <test-instance>

For more kitchen commands, execute

    kitchen help

## Test Coverage

Tests are composed of platforms and suites. A suite is a series of commands to
execute (usually in the form of Chef recipes) followed by a series of tests
that verify that the commands completed as expected. Platforms are
systems environments in which the suites are executed. Each test suite is
executed on each platform.

### Platforms Tested:

See [kitchen-azure.yml](kitchen-azure.yml)

### Packaging Test Suites

The *release candidate* is the latest Agent built.
In other words, they are the most up-to-date packages on our
staging (datad0g.com) deb (unstable branch) and yum repositories.

Agent packaging methods tested:

- `dd-agent`: Installing the latest release candidate using the
  [chef-datadog](https://github.com/DataDog/chef-datadog). The recipe will
  determine if the base Agent should be installed instead of the regular one
  (based on the system version of Python).
- `dd-agent-upgrade-agent6`: Installs the latest release Agent 6 (the latest publicly
  available Agent 6 version), then upgrades it to the latest release candidate
  using the platform's package manager.
- `dd-agent-upgrade-agent5`: Installs the latest release Agent 5 (the latest publicly
  available Agent 5 version), then upgrades it to the latest Agent 6 release candidate
  using the platform's package manager.
- `dd-agent-install-script`: Installs the latest release candidate using our [Agent
  install script](https://raw.github.com/DataDog/dd-agent/master/packaging/datadog-agent/source/install_agent.sh).
- `dd-agent-step-by-step`: Installs the latest release candidate using our
  step-by-step installation instructions as listed on Dogweb.

For each platform, for each packaging method, the following Agent tests are
run (in the order listed):

  * The Agent is running:
    - the Agent is running after installation is complete
    - the configuration has been correctly placed (where applicable)
    - the info command output does not contain 'ERROR' and exits with a non-zero
      value
  * The Agent stops:
    - the `stop` command does not error when the Agent is running
    - no Agent processes are running after the Agent has been stopped
    - the Agent starts after being stopped
  * The Agent restarts:
    - the `restart` command does not error when the Agent is running
    - the Agent is running after a previously-running Agent it is restarted
    - the `restart` command does not error when the Agent is stopped
    - the Agent is running after a previously-stopped Agent it is restarted
  * The Agent is removed (if installed using a package manager):
    - removing the Agent using the package manager should not error
    - the Agent should not be running after it is removed
    - the Agent binary should not be present after removal

For the `dd-agent-upgrade` method, the version of the agent after the upgrade is tested.
Be sure to set the `DD_AGENT_EXPECTED_VERSION` environment variable to the version the agent
should be upgraded to (for instance `export DD_AGENT_EXPECTED_VERSION='5.5.0+git.213.59ac9da'`).

### Agent Properties that are NOT Tested

This suite covers the installing, upgrading, and removing the Agent on most
platforms. The following, among other aspects, are *not* covered:

* Updates from versions of the Agent prior to the latest publicly available
  version. This can easily be added by setting the ['datadog']['agent_version']
  attribute to the version that you wish to upgrade from in the
  `dd-agent-upgrade` suite
* Not all supported operating systems are tested. Missing operating systems
  include:
    - Mac OS 10.x
    - Amazon Linux
* Memory leaks or any obscurities that result from the Agent running for a long time, and so short term tests like these will not catch them
* Sending metrics/events over Dogstatsd via client libraries
* Changes made to an environment by a user (no sudo present, system Python
  removed, supervisor installed etc.)

## Adding Tests

For basic test-kitchen usage, see its [Getting Started
guide](https://github.com/opscode/test-kitchen/wiki/Getting-Started).

### Adding a Platform
Tests are executed on Azure. Platforms are defined in `kitchen-azure.yml`.

### Adding a Suite
Suites define the commands that should be run on a platform as Chef recipes,
and tests that verify the outcome of these commands written in RSpec. Suites
are defined in `kitchen-azure.yml`.

Add new cookbooks by describing them in `Berksfile`. If you want to write your
own cookbook that is specific to this repository, place it in the
`site-cookbooks` directory. New cookbooks will not be available in your tests
until `rake berks` is executed.

Tests should be placed in `test/integration/<suite-name>/rspec/`. They will be
copied and executed on a platform after the suite's recipes have been applied
to the environment. They are not managed by `Berks`. Code that can be shared
between suites is in `test/integration/common/rspec/`.

Tests do not need to be written in RSpec; [Busser](https://github.com/fnichol/busser),
the framework that executes the tests, supports several different testing
frameworks (including Bats and minitest).
