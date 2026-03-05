# Yash automated test

This directory includes automated test cases for yash.

The test cases are grouped into POSIX tests and yash-specific tests, which
are written in files named `*-p.tst` and `*-y.tst`, respectively. Every POSIX
shell is supposed to pass the POSIX tests, so those test cases does not
test any yash-specific behavior at all. To run the POSIX tests on a shell
other than yash, run in this directory:

    $ make TESTEE=<shell_command_name> test-posix

---------------------------------------------------------------------------

Some test cases are skipped by the test runner depending on the
configuration of yash, user's privilege, etc. If the help built-in is
disabled in the configuration, for example, tests for the help built-in are
skipped. There is no configuration in which no tests are skipped; some
tests require a root privilege while some require a non-root privilege.

---------------------------------------------------------------------------

Test cases can be run in parallel if your make supports parallel build.
Exceptionally, test cases that require a control terminal have to be run
sequentially if a pseudo-terminal cannot be opened to obtain a control
terminal that can be used for testing. In that case, you have to run the
tests in the foreground process group so that they can make use of the
current control terminal. Test case files containing such tests are marked
with the `%REQUIRETTY%` keyword.

---------------------------------------------------------------------------

To test yash with Valgrind, run `make test-valgrind` in this directory.
Yash must have been built without enabling any of the following variables
in `config.h`:

 * `HAVE_PROC_SELF_EXE`
 * `HAVE_PROC_CURPROC_FILE`
 * `HAVE_PROC_OBJECT_AOUT`

Otherwise, some tests would fail after Valgrind is invoked as a shell where
yash should be invoked.

Some tests are skipped to avoid false failures.

## Writing test cases

Test cases are written as shell scripts that use the test harness provided in
`run-test.sh`. This harness provides functions and aliases to define individual
test cases.

Each test case is defined by one of the `test_*` aliases. These aliases execute
the shell being tested with a given script and compare the standard output,
standard error, and exit status against expected values.

The general format of a test case is:

```sh
test_<outputs> [options] <name> <shell_arguments...>
<standard input for the shell>
__IN__
<expected standard output>
__OUT__
<expected standard error>
__ERR__
```

### Test Harness Aliases

The `<outputs>` part in the alias name specifies which outputs to check:

* `test_x`: No output is checked.
* `test_o`: Only standard output is checked.
* `test_e`: Only standard error is checked.
* `test_oe`: Both standard output and standard error are checked.

You can also specify that an output should be empty by using a capital letter:

* `test_O`: Expect empty standard output.
* `test_E`: Expect empty standard error.
* `test_OE`: Expect both standard output and standard error to be empty.
* `test_oE`: Check standard output and expect empty standard error.
* `test_Oe`: Expect empty standard output and check standard error.

### Test Case Options

The `testcase` function, which is called by the aliases, accepts several
options:

* `-e <status>`: Check the exit status. `<status>` can be a number, `n` for any
  non-zero status, or a signal name (e.g., `INT`).
* `-d`: Assert that the standard error is not empty (for diagnostics).
* `-f`: Expect the test to fail. This is for tests that document known bugs.

Standard input, expected standard output, and expected standard error are
provided using here-documents: `3<<\__IN__`, `4<<\__OUT__`, and `5<<\__ERR__`.
The above aliases redirect these file descriptors to the appropriate streams
for the test.

### Example

Here is an example of a test case that runs `echo "hello"` and checks its
output:

```sh
test_oE -e 0 'simple echo' -c 'echo "hello"'
__IN__
hello
__OUT__
```

This test case:

* Uses `test_oE` to check standard output (`o`) and expect empty standard error
  (`E`).
* Uses `-e 0` to expect an exit status of 0.
* Is named `'simple echo'`.
* Runs the command `echo "hello"` in the testee shell (using `-c`).
* Provides no standard input (the `__IN__` here-document is empty).
* Expects `hello` on standard output.

The test harness script `run-test.sh` contains the full implementation and
further details.
