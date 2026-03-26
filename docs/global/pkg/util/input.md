# pkg/util/input

## Purpose

Thin wrapper around `bufio.Scanner` that reads interactive user input from `stdin` during CLI operations (flare submission, confirmation prompts). It is intentionally minimal with no external dependencies.

## Key elements

| Function | Description |
|---|---|
| `AskForEmail() (string, error)` | Prints `"Please enter your email: "` and returns the trimmed line read from stdin. |
| `AskForConfirmation(prompt string) bool` | Prints the caller-supplied prompt, reads one line, and returns `true` only if the response is `"y"` or `"Y"`. Returns `false` on any scanner error. |

Both functions are backed by the unexported `askForInput(before, after string)` which uses a `bufio.Scanner` on `os.Stdin`, trims whitespace from the result, and optionally prints a suffix string after reading.

## Usage

Used exclusively in CLI subcommands that require user interaction before performing a potentially destructive or network-bound action:

- `cmd/agent/subcommands/flare` — asks for email and send-confirmation before uploading a flare.
- `cmd/security-agent/subcommands/flare` — same pattern.
- `cmd/otel-agent/subcommands/flare` — same pattern.
- `pkg/cli/subcommands/dcaflare` — cluster-agent flare.
- `cmd/agent/subcommands/dogstatsdstats` — confirmation before collecting DogStatsD stats.

```go
import "github.com/DataDog/datadog-agent/pkg/util/input"

email, err := input.AskForEmail()
if err != nil {
    return err
}
if !input.AskForConfirmation("Send flare? [y/N] ") {
    return nil
}
```

## Notes

- There is no timeout or context support; the scanner blocks until the user presses Enter.
- The package has no build constraints and works on all platforms.
