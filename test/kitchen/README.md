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
```
bundle config set --local path '.'
bundle config set --local gemfile './Gemfile.local'
```

`bundle install`

Note: When building on macOS M1, you might run into an error when installing the `ffi-yajl` gem. You should be able to get around that by setting the build `ldflags` for this gem in bundler configuration (see [this Github Issue](https://github.com/chef/ffi-yajl/issues/115)):
```bash
bundle config build.ffi-yajl --with-ldflags="-Wl,-undefined,dynamic_lookup"
```

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

To generate a test suite, the platforms to be tested are passed in via the
TEST_PLATFORMS environment variable.  The exact format of the list that's
supplied in this variable will vary depending upon which driver (provider)
is being used.

In Azure (which is what is used in the ci pipeline), the list of platforms
is created in the top level .gitlab-ci.yaml, and the syntax is of the form:
`short_name1,azure_full_qualified_name1|short_name2,azure_full_qualified_name2`

Each driver (provider) will have a slightly different format, which is described
in the driver-specific yaml (drivers/azure-driver.yml, drivers/ec2-driver, etc.)


### Packaging Test Suites

The *release candidate* is the latest Agent built.
In other words, they are the most up-to-date packages on our
staging (datad0g.com) deb (unstable branch) and yum repositories.

Agent packaging methods tested:

- `chef`: Installing the latest release candidate using the
  [chef-datadog](https://github.com/DataDog/chef-datadog). The recipe will
  determine if the base Agent should be installed instead of the regular one
  (based on the system version of Python).
- `upgrade-agent6`: Installs the latest release Agent 6 (the latest publicly
  available Agent 6 version), then upgrades it to the latest release candidate
  using the platform's package manager.
- `upgrade-agent5`: Installs the latest release Agent 5 (the latest publicly
  available Agent 5 version), then upgrades it to the latest Agent 6 release candidate
  using the platform's package manager.
- `install-script`: Installs the latest release candidate using our [Agent
  install script](https://raw.github.com/DataDog/dd-agent/master/packaging/datadog-agent/source/install_agent.sh).
- `step-by-step`: Installs the latest release candidate using our
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

For the `upgrade` method, the version of the agent after the upgrade is tested.
Be sure to set the `DD_AGENT_EXPECTED_VERSION` environment variable to the version the agent
should be upgraded to (for instance `export DD_AGENT_EXPECTED_VERSION='5.5.0+git.213.59ac9da'`).

### Agent Properties that are NOT Tested

This suite covers the installing, upgrading, and removing the Agent on most
platforms. The following, among other aspects, are *not* covered:

* Updates from versions of the Agent prior to the latest publicly available
  version. This can easily be added by setting the ['datadog']['agent_version']
  attribute to the version that you wish to upgrade from in the
  `upgrade` suite
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
Tests are executed on Azure in ci. Platforms are defined in the OS dependent
files kitchen testing definitions in /.gitlab/kitchen_testing/<os>.yaml

### Adding a Suite
Suites define the commands that should be run on a platform as Chef recipes,
and tests that verify the outcome of these commands written in RSpec. Suites
are defined in specific yaml files in the `test-definitions` directory.

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

### Creating a complete test file (kitchen.yaml)

To support running multiple kitchen suites using multiple back-end providers, the
kitchen configuration file (kitchen.yaml) is created dynamically by combining three
provided yaml files.

A complete kitchen configuration (kitchen.yaml) is created by taking one of the
driver files, adding the common configuration options [platforms-common](test-definitions/platforms-common.yml),
and then adding the desired test suite(s) from the test-definitions directory.

There is an invoke task for creating the completed `kitchen.yml`.  The usage for
the invoke task is:

  invoke kitchen.genconfig --platform=platform --osversions=osversions --provider=provider --arch=arch --testfiles=testfiles

where:
  platform is an index into the platforms.json.  It is (currently) one of:
  - centos
  - debian
  - suse
  - ubuntu
  - windows
  - amazonlinux

  osversions is an index into the per-provider dictionary for the given
  platform.  Examples include
  - win2012r2 (which is in both windows/azure and windows/ec2)
  - ubuntu-20-04 (which is in ubuntu/azure)

  Provider is the kitchen driver to be used.  Currently supported are
  - azure
  - ec2
  - vagrant
  - hyperv (with a user-supplied platforms file)

  arch is:
  - x86_64
  - arm64

  Testfiles is the name of the test-specific file(s) (found in [test-definitions](test-definitions) ) to be
  added.  The testfiles define the tests that are run, on what OS, on the given provider.

An example command would be
  invoke kitchen.genconfig --platform ubuntu --osversions all --provider azure --arch x86_64 --testfiles install-script-test

  This will generate a kitchen.yml which executes the `install-script-test` on all of the defined `ubuntu`
  OS images in azure.

#### Running in CI

When run in CI, the gitlab job will set up the `TEST_PLATFORMS` environment variable,
and then concatenate the [azure driver](drivers/azure-driver.yml), [platforms-common](test-definitions/platforms-common.yml),
and the desired test suite file (for example [upgrade7-test](test-definitions/upgrade7-test.yml)).

Test kitchen then expands out each of the `TEST_PLATFORMS` into a kitchen platform, and runs
each suite on each provided platform

#### Running locally

To run kitchen locally, either to do broad tests on a given build or to develop the
kitchen tests themselves, additional driver files are provided for using AWS EC2, Hyper-V,
or vagrant as the back end.

To create a kitchen file, take the same steps as above.  Combine the desired driver file,
the common file, and the desired test suite(s). Using `erb` as a manual step will generate
a kitchen file that can be reused.

At present, the EC2 and Hyper-V driver files have only been tested for Windows targets.
