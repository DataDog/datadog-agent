# Windows NPM Test definitions

## Test matrix

The tests in this test file attempt to test all of the install/upgrade scenarios for installing the Windows agent with various install options.  Starting in 7.45, the installation option changed from a Windows feature (NPM) to a more general "allow closed source".

For these tests, then, installing/upgrading from an "old" version means <= 7.43.
Installing/upgrading from a "previous" version means > 7.43, but less than current version. (this is different for testing reacting
to the way the new state is recorded).
The "current" version is the version in test.


Install scenarios expected
1. Install old version with no NPM flag, install new version
    - expect driver installed, system probe to enable & start
2. Install old version with no NPM flag, install new version
    - expect driver installed, disabled
3. Install old version with NPM flag, install new version
    - expect install to detect NPM previously installed, results in system probe enabling/starting driver
4. Install new version with NPM disabled
    - expect driver installed, disabled
5. Install new version with NPM enabled
    - expect driver installed, system probe to enable & start
7. Install version with no flag, reinstall same version with ADDLOCAL=ALL
    - expect previous setting to be maintained (driver installed, system probe starts it)
8.  Install version with ADDLOCAL=ALL
    - (driver installed, system probe starts it)
9.  Install version with ADDLOCAL=NPM

## win-npm-upgrade-to-npm
Scenario 1

## win-npm-upgrade-no-npm
Scenario 2

## win-npm-upgrade-to-npm-no-csflag
Scenario 3

## win-npm-no-npm-option
Scenario 4

## win-npm-with-cs-option
Scenario 5

## Scenario 6 not currently enabled

## win-npm-reinstall-option
Scenario 7

## win-npm-with-addlocal-all
Scenario 8

## win-npm-with-addlocal-npm
Scenario 9
