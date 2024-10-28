# Testing Best Practices

This document describes some best-practices for unit testing in the Agent.
Please feel invited to:
 * Refer to this document in pull requests where these practices might be helpful
 * Add best practices to this document
 * Refactor tests to follow these best practices
 * Change practices if they are no longer the best

**Note**: The code will never completely reflect these practices, although we hope to get continually closer.

## [Go] Use `require` Instead of `assert` When Necessary

The functions in `github.com/stretchr/testify/require` automatically abort the test when an assertion fails, whereas `github.com/stretchr/testify/assert` does not.

For example, given an error, `assert.NoError(t, err)` causes the test to be marked as a failure, but continues to the next statement, possibly leading to a nil dereference or other such failure.
In contrast, `require.NoError(t, err)` aborts the test when an error is encountered.

Where a test makes a sequence of independent assertions, `assert` is a good choice.
When each assertion depends on the previous having been successful, use `require`.

## [Go] Testing Timing-Related Functionality

Tests based on time are a major source of intermittents.
If you find yourself thinking something like "the ticker should run three times in 500ms", you will be disappointed at how often that is not true in CI.
Even if that test is not intermittent, it will take at least 500ms to run.
Summing such delays over thousands of tests means _very_ long test runs and slower work for everyone.

When the code you are testing requires time, the first strategy is to remove that requirement.
For example, if you are testing the functionality of a poller, factor the code such that the tests can call the `poll()` method directly, instead of waiting for a Ticker to do so.

Where this is not possible, refactor the code to use a Clock from https://pkg.go.dev/github.com/benbjohnson/clock.
In production, create a `clock.Clock`, and in tests, inject a `clock.Mock`.
When time should pass in your test execution, call `clock.Add(..)` to deterministically advance the clock.

A common pattern for objects that embed a timer is as follows:

```go
func NewThing(arg1, arg2) *Thing {
    return newThingWithClock(arg1, arg2, clock.New())
}

func newThingWithClock(arg1, arg2, clock clock.Clock) *Thing {
    return &Thing{
        ...,
        clock: clock,
    }
}

func TestThingFunctionality(t *testing.T) {
    clk := clock.NewMock()
    thing := newThingWithClock(..., clk)

    // ...

    clk.Add(100 * time.Millisecond)

    // ...
}
```
