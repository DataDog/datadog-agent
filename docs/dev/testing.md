# Guidelines For Testing

TBD

## Testing Timing-Related Functionality

Tests based on time are a major source of intermittents.
If you find yourself thinking something like "the ticker should run three times in 500ms", you will be disappointed at how often that is not true in CI.
Even if that test is not intermittent, it will take at least 500ms to run.
Summing such delays over thousands of tests means _very_ long test runs and slower work for everyone.

When the code you are testing requires time, the first strategy is to remove that requirement.
For example, if you are testing the functionality of a poller, factor the code such that the tests can call the `poll()` method directly, instead of waiting for a Ticker to do so.

Where this is not possible, refactor the code to use a Clock from https://pkg.go.dev/github.com/benbjohnson/clock.
In production, create a `clock.Clock`, and in tests, inject a `clock.Mock`.
When time should pass in your test execution, call `clock.Add(..)` to deterministically advance the clock.
