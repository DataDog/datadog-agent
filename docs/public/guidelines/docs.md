# Writing developer docs

This site is built by [MkDocs](https://github.com/mkdocs/mkdocs) and uses the [Material for MkDocs](https://squidfunk.github.io/mkdocs-material/) theme.

You can serve documentation locally with the `docs.serve` [invoke task](../setup.md#Preface).

## Organization

The site structure is defined by the [`nav`](https://www.mkdocs.org/user-guide/configuration/#nav) key in the [`mkdocs.yml`](https://github.com/DataDog/datadog-agent/blob/main/mkdocs.yml) file.

When adding new pages, first think about what it is _exactly_ that you are trying to document. For example, if you intend to write about something everyone must follow as a standard practice it would be classified as a guideline whereas a short piece about performing a particular task would be a how-to.

After deciding the kind of content, strive to further segment the page under logical groupings for easier navigation.

## Line continuations

For prose where the rendered content should have no line breaks, always keep the Markdown on the same line. This removes the need for any stylistic enforcement and allows for IDEs to intelligently wrap as usual.

!!! tip
    When you wish to force a line continuation but stay within the block, indent by 2 spaces from the start of the text and end the block with a new line. For example, the following shows how you would achieve a multi-line ordered [list item](https://spec.commonmark.org/0.31.2/#list-items):

    <div class="grid cards" markdown>

    === "Markdown"

        ```markdown
        1. first line

             second line

        1. third line
        ```

    ===! "Rendered"

        1. first line

             second line

        1. third line

    </div>

## Emphasis

When you want to call something out, use [admonitions](https://squidfunk.github.io/mkdocs-material/reference/admonitions/) rather than making large chunks of text bold or italicized. The latter is okay for small spans within sentences.

Here's an example:

<div class="grid cards" markdown>

-   Markdown

    ---

    ```markdown
    !!! info
        Lorem ipsum ...
    ```

-   Rendered

    ---

    !!! info
        Lorem ipsum ...

</div>

## Links

Always use [inline links](https://spec.commonmark.org/0.31.2/#inline-link) rather than [reference links](https://spec.commonmark.org/0.31.2/#reference-link).

The only exception to that rule is links that many pages may need to reference. Such links may be added to [this file](https://github.com/DataDog/datadog-agent/blob/main/docs/public/.snippets/links.txt) that all pages are able to reference.

## Abbreviations

[Abbreviations](https://squidfunk.github.io/mkdocs-material/reference/tooltips/#adding-abbreviations) like DSD may be added to [this file](https://github.com/DataDog/datadog-agent/blob/main/docs/public/.snippets/abbrs.txt) which will make it so that a tooltip will be displayed on hover.
