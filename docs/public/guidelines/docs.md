# Writing developer docs

-----

This site is built by [Zensical](https://zensical.org), using its classic theme variant for compatibility with Material for MkDocs.

You can serve documentation locally with the `dda run docs serve` command.

Once the site is built with `dda run docs build`, `dda run docs check-links` resolves every link it contains, which `dda run docs build --check` does for you in one step.

## Organization

The site structure is defined by the [`nav`](https://zensical.org/docs/setup/navigation/) key in the <<<repo("mkdocs.yml")>>> file.

We strive to follow the principles of the Diátaxis [documentation framework](https://diataxis.fr).

When adding new pages, first think about what it is _exactly_ that you are trying to document. For example, if you intend to write about something everyone must follow as a standard practice it would be classified as a guideline whereas a short piece about performing a particular task would be a how-to.

After deciding the kind of content, further segment the page under logical groupings for easier navigation.

## Ordered lists

Each item in an [ordered list](https://spec.commonmark.org/0.31.2/#ordered-list-marker) should start with `1.` and let rendering handle the rest. This is recommended for two reasons:

1. Changes to the list size do not require re-numbering unmodified items and therefore reduces the diff when reviewing.
1. Rendering will expose improper formatting by having the sequence broken rather than hiding such issues.

## Line continuations

For prose where the rendered content should have no line breaks, always keep the Markdown on the same line. This removes the need for any stylistic enforcement and allows for IDEs to intelligently wrap as usual.

/// tip
When you wish to force a line continuation but stay within the block, indent by 2 spaces from the start of the text and end the block with a new line. For example, the following shows how you would achieve a multi-line ordered [list item](https://spec.commonmark.org/0.31.2/#list-items):

<div class="grid cards" markdown>

-   Markdown

    ---

    ```markdown
    1. first line

         second line

    1. third line
    ```

-   Rendered

    ---

    1. first line

         second line

    1. third line

</div>

///

## Emphasis

When you want to call something out, use [admonitions](https://squidfunk.github.io/mkdocs-material/reference/admonitions/) rather than making large chunks of text bold or italicized. The latter is okay for small spans within sentences.

Here's an example:

<div class="grid cards" markdown>

-   Markdown

    ---

    ```markdown
    /// info
    Lorem ipsum ...
    ///
    ```

-   Rendered

    ---

    /// info
    Lorem ipsum ...
    ///

</div>

## Links

Always use [inline links](https://spec.commonmark.org/0.31.2/#inline-link) rather than [reference links](https://spec.commonmark.org/0.31.2/#reference-link).

The only exception to that rule is links that many pages may need to reference. Such links may be added to <<<repo("docs/public/.snippets/links.txt", "this file")>>> that all pages are able to reference.

### Repository paths

Never write a link to a file or directory of this repository by hand. Give the `repo` macro a path relative to the repository root and it links files as `blob`, links directories as `tree`, and fails the build when the path no longer exists. Links to anything else the repository hosts, such as a pull request, are still written by hand.

Links point at the branch being built rather than always at `main`, so a preview built from a pushed branch links to that branch's own files.

<div class="grid cards" markdown>

-   Markdown

    ---

    ```markdown
    <<% raw %>>- Directory

        <<<repo("bazel/rules")>>>

    - File

        <<<repo("MODULE.bazel")>>>

    - Line in file

        <<<repo(
            ".gitlab-ci.yml",
            match="^stages:",
        )>>><<% endraw %>>
    ```

-   Rendered

    ---

    - Directory

        <<<repo("bazel/rules")>>>

    - File

        <<<repo("MODULE.bazel")>>>

    - Line in file

        <<<repo(
            ".gitlab-ci.yml",
            match="^stages:",
        )>>>

</div>

The link text defaults to the path in backticks. A second argument replaces it and is itself Markdown, so add backticks yourself when you want a short monospaced label.

To link a single line, pass `match` with a regular expression that matches exactly one line of the file, as above. The search is unanchored, so `^` and `$` are available to pin the expression to the start or end of a line, and characters such as `(` need escaping to match literally. The line number is resolved on every build and therefore never drifts, so never write one by hand.

When a link will not do, such as in a raw HTML attribute, `repo_url` validates the same path and `match` arguments but renders only the URL:

<div class="grid cards" markdown>

-   Markdown

    ---

    ```markdown
    <<% raw %>><a href="<<<repo_url(
        ".gitlab-ci.yml",
        match="^stages:",
    )>>>">the stage list</a><<% endraw %>>
    ```

-   Rendered

    ---

    <a href="<<<repo_url(
        ".gitlab-ci.yml",
        match="^stages:",
    )>>>">the stage list</a>

</div>

The macro rejects Markdown pages under `docs/public`, which are linked relatively so that readers stay on this site.

/// warning
Macros are rendered everywhere on a page, including inside fenced code blocks. To show a macro rather than call it, wrap it in `<<<"<<% raw %>>">>>` and `<<<"<<% endraw %>>">>>` on the same line, the way the example above does. Putting those tags on their own lines leaves blank lines behind.

Macros do not run inside <<<repo("docs/public/.snippets")>>>, whose files are appended to each page after macros have already been rendered.
///

## Abbreviations

[Abbreviations](https://squidfunk.github.io/mkdocs-material/reference/tooltips/#adding-abbreviations) like DSD may be added to <<<repo("docs/public/.snippets/abbrs.txt", "this file")>>> which will make it so that a tooltip will be displayed on hover.
