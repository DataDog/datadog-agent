# Builtin Commands Reference

All builtin commands available in the shell interpreter. External commands are blocked by
default and require an `ExecHandler` to be configured.

## Summary

| Command    | Synopsis            | Description                          |
|------------|---------------------|--------------------------------------|
| `echo`     | `echo [ARG...]`     | Print arguments to stdout            |
| `cat`      | `cat [FILE...]`     | Concatenate files to stdout          |
| `true`     | `true`              | Return success                       |
| `false`    | `false`             | Return failure                       |
| `exit`     | `exit [N]`          | Exit the shell                       |
| `break`    | `break [N]`         | Break out of a loop                  |
| `continue` | `continue [N]`      | Skip to next loop iteration          |

## echo

```
echo [ARG...]
```

Print arguments separated by spaces, followed by a newline.

- With no arguments, prints an empty line.
- No options are supported (`-n`, `-e`, etc. are treated as literal arguments).
- Exit code: always **0**.

## cat

```
cat [FILE...]
```

Read files and write their contents to stdout. Respects `AllowedPaths` restrictions.

- With no arguments, reads from stdin.
- `-` reads from stdin explicitly.
- Files are read in order; contents are concatenated.
- Exit code: **0** on success, **1** if any file cannot be read.

## true

```
true [ARG...]
```

Do nothing, successfully. Arguments are ignored.

- Exit code: always **0**.

## false

```
false [ARG...]
```

Do nothing, unsuccessfully. Arguments are ignored.

- Exit code: always **1**.

## exit

```
exit [N]
```

Exit the shell with status code N.

- With no argument, exits with the last command's exit code.
- N is interpreted as an unsigned 8-bit integer (values wrap modulo 256).
- Invalid argument: prints error, exits with code **2**.
- Multiple arguments: prints error, exits with code **1**.
- The shell always terminates, even when the argument is invalid.

## break

```
break [N]
```

Exit from N enclosing `for` loops. Defaults to 1.

- Outside a loop: prints a warning to stderr, returns **0**, execution continues.
- N < 1: prints error, returns **1**.
- Non-numeric N: prints error, returns **2**, still breaks 1 level.
- Multiple arguments: prints usage, returns **2**, still breaks 1 level.
- If N exceeds loop nesting depth, breaks out of all enclosing loops.

## continue

```
continue [N]
```

Skip to the next iteration of the Nth enclosing `for` loop. Defaults to 1.

- Outside a loop: prints a warning to stderr, returns **0**, execution continues.
- N < 1: prints error, returns **1**.
- Non-numeric N: prints error, returns **2**, still breaks 1 level.
- Multiple arguments: prints usage, returns **2**, still breaks 1 level.
- If N exceeds loop nesting depth, continues from the outermost loop.
