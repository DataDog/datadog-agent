## Useful Prompts

### Prompt for generating test scenarios:
```
Improve tests scenarios coverage by taking inspiration from yash_posix_tests

Notes:
- avoid duplicate test coverage, if you encounter duplicate scenarios, remove or merge them
- create as many new scenarios as possible (no limit), of course they must be valuable
- if some tests fail, keep in mind it's possible that the shell implementation is wrong, and it's fine to fix the implementation
```

### Prompt to avoid disabling skip_assert_against_bash when possible:
```
In tests, for each scenarios with skip_assert_against_bash is disabled,
examine why skip_assert_against_bash is disabled.

skip_assert_against_bash is usually disabled because the restricted shell doesn't behave as bash

BUT if it's disabled because of a bug in restricted shell (making it behave differently than bash),
then the restricted shell implementation must be fixed
```

### Prompt for implementing a builtin

```
Implement the POSIX cat command following docs/RULES.md and the existing builtins.

Ask me for clarification before changing behavior.
```

## Tips 

Good sources of POSIX shell test scenarios:
- [yash POSIX shell test suite](https://github.com/magicant/yash/tree/trunk/tests)
