## Test Kitchen Overview

### Build pipeline testing

#### Configuration (.gitlab-ci.yml)

The gitlab pipelines which require kitchen testing have 3 independent steps for provisioning kitchen testing, running kitchen testing, and cleaning up (removing the artifacts).  The _deploy\_\<platform\>\_testing_ tasks copy the build artifacts to an S3 bucket.  The _kitchen\_\<platform\>_ tasks execute the kitchen tests.  The _testkitchen\_cleanup\_\<cloud_provider\>_ tasks clean up the s3 buckets and the azure virtual machines.

##### Test execution

Each individual test environment is controlled from _.gitlab\_ci.yml_.  The tests are set up using an environment variable, which is a list of test platforms which are turned into test kitchen platforms.  In _.gitlab\_ci.yml_, the platform list has the following syntax:
        `short_name1,azure_full_qualified_name1|short_name2,azure_full_qualified_name2`

This configuration is fed to the script _tasks/run-test-kitchen.sh_. That script in turn sets up additional variables for Azure (authentication/environment variables), and then executes the actual kitchen command.  Using the ruby _erb_ syntax, a full matrix of platforms and tests is generated and executed.

There are currently two input files.
1. _kitchen-azure.yml_ contains a set of installation and upgrade tests that are executed on all of the available platforms
2. _kitchen-azure-winstall.yml_ contains a set of  Windows specific tests for command-line installation options. These tests are currently (as configured in _.gitlab-ci.yml_) only run on one Windows OS for brevity.

### Test Implementation

#### Directory structure

Test kitchen imposes a very specific directory structure.  For more information, see the test kitchen documentation.  However, a brief overview:

The _site\_cookbooks_ tree contains the chef cookbooks that are run during the _kitchen converge_ stage.  
```
.
├── site-cookbooks
│   ├── dd-agent-install
│   │   ├── attributes
│   │   │   └── default.rb
│   │   ├── recipes
│   │   │   ├── _agent6_windows_config.rb
│   │   │   ├── default.rb
│   │   │   ├── _install_windows_base.rb
│   │   │   └── _install_windows.rb
│   ├── dd-agent-install-script
│   │   ├── attributes
│   │   │   └── default.rb
│   │   ├── recipes
│   │   │   └── default.rb
```
etc.

The cookbooks are run prior to an individual test.  Each listed cookbook is run before any verification stage, so multi-stage tests can't do verification in between stages (like the upgrade test).

The _test/integration_ tree contains the _rspec_ verification tests that are run during the _verify_ stage.

```
└── test
    └── integration
        ├── common
        │   └── rspec
        │       └── spec_helper.rb
        ├── dd-agent
        │   └── rspec
        │       ├── dd-agent_spec.rb
        │       └── spec_helper.rb -> ../../common/rspec/spec_helper.rb
        ├── dd-agent-all-subservices
        │   └── rspec
        │       ├── dd-agent-all-subservices_spec.rb
        │       └── spec_helper.rb -> ../../common/rspec/spec_helper.rb
        ├── dd-agent-installopts
        │   └── rspec
        │       ├── dd-agent-installopts_spec.rb
        │       └── spec_helper.rb -> ../../common/rspec/spec_helper.rb
```
Note that each directory contains a symbolic link for spec_helper.rb to the common directory.  The symbolic link is necessary because chef/kitchen will copy the test files from the test directory only to the target machine.  To prevent copy/paste, and make maintenance manageable, therefore there's a group of common test definitions in _spec\_helper.rb_ that is shared via the symbolic links.

**On Windows** symbolic links must be explicitly enabled in git prior to fetching the tree.  This can be done at install time, or after the fact with `git config --global core.symlinks=true`

#### Test definition

The test suites are defined in the various _.kitchen-*.yml_ files.  Each test suite runs one or more chef recipes (usually to install the agent in various ways), and then its verifier.  The verifier is assumed (by kitchen) to be in the directory that has the name of the suite.  So, the `dd-agent-installopts` suite runs the verifier script in `dd-agent-installopts/rspec/dd-agent-installopts_spec.rb`.