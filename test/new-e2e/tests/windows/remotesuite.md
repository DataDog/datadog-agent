# the RemoteSuite library

The remotesuite library attempts to capture and duplicate the functionality for running locally compiled go test programs on remote test machines.

## Background

There are several tests that cannot be run directly as unit tests during CI.  This is due to various test requirements (such has having a driver installed).
Historically, this has been accomplished by using chef/test kitchen as follows:

- a previous build stage creates the various test programs.  The test programs are still `go` tests, but they are pre-compiled into binary form, generally into an executable called `testsuite.exe`
- the test stage uses test/kitchen to provision a target machine, set up the target machine with relevant components, copy the necessary files, and execute the tests on the target machine.

In order to minimize churn on the environment, the `remotesuite` assumes that the existing (chef/kitchen) directory structure is still in place.  This will lead to some fairly obvious changes/optimizations at a later date if/when the kitchen infrastructure is retired completely.

# RemoteSuite

The RemoteSuite breaks up operations into individual stages.  While some of these could be combined, they are intentionally distinct, to allow greater control for the test writer.  Also, some of the actions happen locally on the test runner, and some on the remote test host.  The objective is to (eventually) be able to run the local actions once, and then use the results on each target host when testing more than one.

## FindTestPrograms

FindTestPrograms() takes the base path, and walks the entire directory structure, looking for `testsuite.exe`.  This is owing to the existing `chef` based directory structure.  

## CreateRemotePaths

Using the result of FindTestPrograms(), this stage makes a duplicate directory structure on the target/test host.  This stage is intended to be able to be replicated on more than one host.

## CopyFiles

This stage copies each testsuite executable to the target host, preserving the relative directory structure.  It also makes the assumption that if there is an adjacent `testdata` directory, that it should be copied too.  This is to conform to the practice in place in the kitchen based tests that support files would be in an adjacent `testdata`.  The tests rely on that directory being present, if necessary.

## RunTests

Run tests walks each testsuite and executes it, capturing the output.  

# Future/optimization

As described in the introduction, much of this work has been done to adhere to the directory strucuture in place for the kitchen/chef based tests.  If those tests are fully retired, and we have the ability to remake the input files, then the module could be simplified.

For example, the two step process of `CreateRemotePaths` and `CopyFiles` could be condensed into a single `vm.CopyFolder()`.  It is not done this way at present, as that would result in copying additional, unnecessary files.