# Set up optional development features

-----

## Tab completion

<<<DDA_DOCS_TAB_COMPLETE>>>

/// tip
There is also limited tab completion support for the legacy `invoke` tasks. Here's an example:

```
echo "source <(dda inv --print-completion-script zsh)" >> ~/.zshrc
```
///

## Pre-commit hooks

The CI runs a number of required checks using [pre-commit](https://pre-commit.com) and running those locally can speed up the development process.

1. Install `pre-commit` by running the following command.

       ```
       dda self pip install pre-commit
       ```

1. Use `dda` to configure the pre-commit hooks.

       ```
       dda inv setup.pre-commit
       ```
