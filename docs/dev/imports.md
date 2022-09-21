# Goimports

In the `imports ( .. )` section of each Go file, imports should be separated into at least three sections:

1. standard library packages
1. external packages
1. local packages

This is not verified or enforced automatically, as doing so is very inefficient.
Instead, please configure your editor to keep imports properly sorted, as described below.

## Editor / IDE Support

The `goimports` tool supports a "local packages" section.  Use the flag `-local github.com/DataDog/datadog-agent`.

Here's how to configure this for a few popular editors.
Please feel free to add instructions for your favorite editor!

### Vim

In vim, using `vim-go`, add

```vim
let g:go_fmt_options = {
\ 'goimports': '-local github.com/DataDog/datadog-agent',
\ }
```

### VSCode

```json
{
  "gopls": {
    "formatting.local": "github.com/DataDog/datadog-agent"
  } 
}
```

See https://github.com/golang/vscode-go/wiki/features#format-and-organize-imports.
