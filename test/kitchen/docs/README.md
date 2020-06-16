# Test Kitchen Overview

## Build pipeline testing

### Configuration (.gitlab-ci.yml)

The gitlab pipelines which require kitchen testing have 3 independent stages for provisioning kitchen testing, running kitchen testing, and cleaning up (removing the artifacts). The `testkitchen_deploy` stage copies the build artifacts to an S3 bucket. The `testkitchen_testing` stage executes the kitchen tests. The `testkitchen_cleanup` stage cleans up the s3 buckets and the azure virtual machines.

#### testkitchen_deploy

This stage contains jobs to deploy the Agent packages created by the package build jobs to our test repositories:
- apttesting.datad0g.com (for .deb packages used by the Debian and Ubuntu tests)
- yumtesting.datad0g.com (for .rpm packages used by the CentOS/RHEL and SUSE tests)
- s3.amazonaws.com/dd-agent-mstesting (for .msi packages used by the Windows tests)

For each package type, there are two jobs: one to deploy the A6 package, the other
to deploy the A7 and IoT packages.

Each pipeline puts its packages in a dedicated branch:
- .deb packages are put in the `dists/pipeline-<PIPELINE_ID>-a6` and `dists/pipeline-<PIPELINE_ID>-a7` branches apttesting.datad0g.com.
- .rpm packages are put in the `pipeline-<PIPELINE_ID>-a6` and `pipeline-<PIPELINE_ID>-a7` branches of yumtesting.datad0g.com.
- SUSE .rpm packages are put in the `suse/pipeline-<PIPELINE_ID>-a6` and `suse/pipeline-<PIPELINE_ID>-a7` branches of yumtesting.datad0g.com.
- .msi packages are put in the `pipelines/A6/<PIPELINE_ID>` and `pipelines/A7/<PIPELINE_ID>` branches of s3.amazonaws.com/dd-agent-mstesting.

#### testkitchen_testing

This stage contains jobs which execute the kitchen tests on our Azure account.

Each job executes one specific test suite, for one OS, for one Agent flavor and one major Agent version.

The `.gitlab-ci.yml` file has been structured to avoid config duplication with the following jobs:

- a common `kitchen_common` job from which all kitchen testing jobs inherit
- `kitchen_agent_aX` jobs that specify the Agent version tested
- `kitchen_os_X` jobs that specify which OS is tested
- `kitchen_test_X` jobs that specify which test suite is run
- `kitchen_datadog_X_flavor` jobs that specify which Agent flavor is run (eg. `datadog-agent`, `datadog-iot-agent`)
- `kitchen_azure_location_X` jobs that specify the Azure location where the tests are run.

**Note:** We spread our kitchen tests on multiple regions mainly because we have a limited number of resources available on each region on Azure.

From these base blocks, we define:
- test types: `kitchen_test_<test_suite>_<flavor>` which is a combination of a chosen test suite, Agent flavor and Azure location.
- scenarios: `kitchen_scenario_<OS>_aX` which is a combination of a chosen OS and major Agent version.

**Note:** To avoid having too many different test types, we bind each test suite with a specific Azure location. Eg. all install script tests are run in the same Azure location.

The real jobs that are run in the Gitlab pipelines are thus a combination of test types and scenarios, named `kitchen_<OS>_<test_suite>_<flavor>-aX`.

##### testkitchen_cleanup

The `testkitchen_cleanup_azure-aX` jobs clean up the Azure VMs that were used during the `testkitchen_testing` stage.

**Note:** The kitchen test jobs send a `destroy` request to Azure when their tests end, so usually these jobs do nothing. They're mainly useful to make sure all VMs have been destroyed, or when kitchen jobs are cancelled while they're running.

**Note:** Azure VMs are also cleaned up on a daily basis by a job in the `mars-jenkins-scripts` pipeline.

The `testkitchen_cleanup_s3-aX` jobs clean up the S3 buckets of the testing repos.

**Important note:** These jobs are always run, even when a kitchen test fails. This is done on purpose (to avoid having remaining resources once a job fails).
The downside of this approach is that when you want to retry a failing kitchen test job, you'll have to re-run the corresponding deploy job first (to put the Agent packages back in the testing repository).

#### Test suites

kitchen tests are defined in a file named `kitchen.yml` which defines:
- the provisioner (`provisioner:` key) used in the tests. In the pipeline, `chef-solo` is used.
- the driver (`driver:` + `driver_config:` keys) used to run the tests. In the pipeline, the [Azure kitchen driver](https://github.com/test-kitchen/kitchen-azurerm) is used.

- the list of platforms (`platforms:` key) used in the tests.
- the list of test suites (`suites:` key) to run.

To reduce duplication, the `kitchen.yml` file is generated at test time by the `run-test-kitchen.sh` script ([see below](#test-execution-tasksrun-test-kitchensh)) by concatenating 2 template files:

- the `kitchen-azure-common.yml` file, which defines the provisioner, driver, and platforms that will be used. It also defines a number of useful variables that can be used by the test suite templates (such as: which major Agent version is used, what are the testing repository URLs, etc.).
- a `kitchen-azure-<test_suite>-test.yml` file, which contains the test suite definition for a particular test. That definition consists in the list of recipes to run, with their attributes, and optionally a list of platforms to not run the test on (eg. if you know that a particular test x platform combination cannot work).

These files are `.erb` template files, the rendered contents (eg. the list of platforms) depend on what variables are passed to the `run-test-kitchen.sh` script.

#### Test execution (tasks/run-test-kitchen.sh)

Each individual test environment is controlled from the `.gitlab-ci.yml` jobs. The tests are set up using environment variables.

##### `TEST_PLATFORMS`

`TEST_PLATFORMS` is a list of test platforms which are turned into test kitchen platforms. In `.gitlab-ci.yml`, the platform list has the following syntax:
        `short_name1,type1,azure_fully_qualified_name1|short_name2,type2,azure_fully_qualified_name2`

There are two types of platforms:
- ones which use a public image (type `urn`): the azure qualified name for such an image can be found by using `az vm image list` to search for the image (using the publisher name, the offer name and/or the sku). The `urn` field contains the azure fully qualified name.
- ones which use a private image (type `id`): the image is in an image gallery in the `kitchen-test-images` resource group of the Azure account. The fully qualified name is the full path to the image in the gallery.

We mainly use private images for platforms we still want to test, but have been removed from the public Azure images list (eg. because they're EOL).

**Note:** this variable is set by the `kitchen_os_X` jobs.

##### `AZURE_LOCATION`

`AZURE_LOCATION` is a string that indicates on which Azure region the test VMs are spun up.

**Note:** this variable is set by the `kitchen_azure_location_X` jobs.

##### `AGENT_FLAVOR`

`AGENT_FLAVOR` is a string that indicates which flavor the Agent is installed (eg. datadog-agent, datadog-iot-agent). In particular, it helps knowing which package has to be installed, and which tests should be applied (eg. doing Python tests on the IoT Agent doesn't make sense).

**Note:** this variable is set by the `kitchen_datadog_X_flavor` jobs.

This configuration is fed to the `tasks/run-test-kitchen.sh`. That script also takes two mandatory parameters:

- an identifier for the test suite that is run (eg. `install-script`, `upgrade5`),
- the major Agent version that is tested (eg. `6` or `7`).

In turn, it sets up additional variables for Azure (authentication/environment variables), as well as some variables used during testing (such as the exact expected Agent version, gathered from the `inv agent.version` command).

It concatenates the `kitchen-azure-common.yml` file with the file specific to the provided test suite into a `kitchen.yml` file, and then executes the actual kitchen command.

### Test Implementation

#### Directory structure

Test kitchen imposes a very specific directory structure. For more information, see the [test kitchen documentation](https://kitchen.ci/docs). However, a brief overview:

##### Cookbooks

The `site_cookbooks` tree contains the chef cookbooks that are run during the `kitchen converge` stage.

**Note:** The test suites can also use recipes from other cookbooks defined in the `Berksfile`, such as our official `chef-datadog` cookbook.

```
.
├── site-cookbooks
│   ├── dd-agent-install-script
│   │   ├── attributes
│   │   │   └── default.rb
│   │   ├── recipes
│   │   │   └── default.rb
```
etc.

The `recipes` directory contains the actions that the cookbook does. By default (ie. when including the `[<cookbook_name>]` recipe to your test suite), the  `default.rb` recipe is used. To target a specific recipe, include `[<cookbook_name>:<recipe_name>]`.

The `attributes` directory contains the default attributes of your recipes. While not mandatory (you can explicitly define all attributes in the test suites), they are useful to quickly know what can be configured in the recipe.

**Note:** The cookbooks are run prior to an individual test. Each listed cookbook is run before any verification stage, so multi-stage tests can't do verification in between stages (like the upgrade test).

##### Tests

The `test/integration` tree contains the `rspec` verification tests that are run during the `kitchen verify` stage.

```
└── test
    └── integration
        ├── common
        │   └── rspec
        │       └── spec_helper.rb
        │       └── iot_spec_helper.rb
        ├── dd-agent-install-script
        │   └── rspec
        │       ├── dd-agent_spec.rb
        │       └── spec_helper.rb -> ../../common/rspec/spec_helper.rb
        │       └── iot_spec_helper.rb -> ../../common/rspec/iot_spec_helper.rb
        ├── dd-agent-all-subservices
        │   └── rspec
        │       ├── dd-agent-all-subservices_spec.rb
        │       └── spec_helper.rb -> ../../common/rspec/spec_helper.rb
        ├── dd-agent-installopts
        │   └── rspec
        │       ├── dd-agent-installopts_spec.rb
        │       └── spec_helper.rb -> ../../common/rspec/spec_helper.rb
```

To prevent copy/paste, and make maintenance manageable, therefore there's a group of common test definitions in `spec_helper.rb` that is shared to the other via the symbolic links. The symbolic link is necessary because chef/kitchen only copies the test files from the specific test directory that will be run to the target machine.

The same is done for IoT-specific helpers, via the `iot_spec_helper.rb` file.

**On Windows** symbolic links must be explicitly enabled in git prior to fetching the tree. This can be done at install time, or after the fact with `git config --global core.symlinks=true`.

The two `spec_helper` files define helpers and groups of examples that can be used by test suites. Test suites can then add their own specific tests in their test file, and/or use the test groups and helpers defined in the `spec_helper` files.

In particular, both `spec_helper` and `iot_spec_helper` have:
- an "install" test group, to check that the Agent is correctly installed.
- a "behavior" test group, to check various properties of the Agent.
- an "uninstall" test group, to uninstall the Agent and verify its uninstall properties (eg. no files left after uninstall).


#### Test definition

Each test suite runs one or more chef recipes (usually to install the agent in various ways), defined by the `run_list:` key, and then its verifier. The verifier is assumed (by kitchen) to be in the directory that has the name of the suite. So, the suite named `dd-agent-install-script` suite runs the verifier script in `dd-agent-install-script/rspec/dd-agent-install-script_spec.rb`.

### How to add a new test suite to the pipeline

1. If needed, create (a) new cookbook(s) that perform(s) the install type you want to test.

- To scaffold your cookbook, you can use the same cookbook structure as existing cookbooks in the directory
- Don't forget to add your cookbook to the `Berksfile`, otherwise Chef won't know that it's there.
- Think of the different attributes that your cookbook needs (usually, it needs a way to know which Agent version it installs, as well as a location where it can fetch the Agent package built by the pipeline), add them to the `attributes/default.rb` file.

2. Create the new test suite file (`kitchen-azure-<suite_name>-test.yml`).

- The most important parts are the run list, the attributes to set for each cookbook used, and optionally a list of platforms to exclude.
- Add the suite to the list of accepted suites in `tasks/run-test-kitchen.sh`.

**Note (HACK):** On some test suites, we define attributes for a cookbook named `dd-agent-rspec`. This is not a real cookbook; we're using this as a workaround to pass variables to the rspec tests (in the rspec tests, we only have access to a `dna.json` file that has the contents of `kitchen.yml`).

3. Create the rspec test directory matching your new test suite.

- Do not forget to create the symlinks to the `rspec_helper` files from the `common` directory if you need them.
- Remember that your main test file must be in `test/integration/<suite_name>/<suite_name>_spec.rb`.

4. Create the necessary Gitlab jobs to run your test suite on all platforms, Agent flavors and versions you want, following the existing naming conventions.
