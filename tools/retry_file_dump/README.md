# Retry file dump

`.retry` files are created by the Agent when the retry queue of the forwarder is full. See `forwarder_storage_max_size_in_bytes` for more information.
This tool dumps the transactions stored in a `.retry` file into a JSON file.

## Build

`bazel build //tools/retry_file_dump`.

## Usage

The following command creates a JSON file (`.retry.json`) for each `.retry` file in `/opt/datadog-agent/run/transactions_to_retry/c47da40ac935c8fd5ca1441a5ee3d068/`:
```
bazel run //tools/retry_file_dump:retry_file_dump -- --folder=/opt/datadog-agent/run/transactions_to_retry/c47da40ac935c8fd5ca1441a5ee3d068/
```

The generated JSON files contain `\ufffdAPI_KEY\ufffd0\ufffd` which is a placeholder for the API key.
