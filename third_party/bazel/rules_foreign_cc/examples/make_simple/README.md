# Simple Make Example

## Dependencies

- clang
- bazel

## Executing the Example

To execute the example, run

```bash
   bazel test ...
```

## Troubleshooting

If you receive an error of the form:

```text
  ccache: FATAL: Failed to create /home/$USER/.ccache/2/f: Read-only file system
```

This is likely because you're have ccache set as your compiler. You can either
disable ccache, or allow the sandbox to write to ~/.ccache:

```bash
  bazel test --sandbox_writable_path ~/.ccache ...
```
