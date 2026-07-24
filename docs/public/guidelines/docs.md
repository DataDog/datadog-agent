# Writing developer docs

-----

This site is built by [Zensical](https://zensical.org), using its classic theme variant for compatibility with Material for MkDocs.

You can serve documentation locally with the `dda run docs serve` command.

## Organization

The developer documentation is made up of two sites that are deployed together:

| Site | Configuration | Content |
| --- | --- | --- |
| [Developer docs](https://datadoghq.dev/datadog-agent/) | [`mkdocs.yml`](https://github.com/DataDog/datadog-agent/blob/main/mkdocs.yml) | `docs/public`, everything about *working on* the Agent |
| [Architecture docs](https://datadoghq.dev/datadog-agent/architecture/) | [`mkdocs.architecture.yml`](https://github.com/DataDog/datadog-agent/blob/main/mkdocs.architecture.yml) | `docs/public/architecture`, everything about *how the Agent works* |

The architecture site is published under the `architecture/` path of the main site, and each site links to the other from its top navigation tabs. Use `dda run docs serve --site architecture` to serve the architecture site locally. Links between the two sites must use absolute URLs (e.g. `https://datadoghq.dev/datadog-agent/architecture/components/overview/`); such links are validated against the freshly built output rather than the deployed sites, so pages added in a pull request may be cross-referenced immediately.

The structure of each site is defined by the [`nav`](https://zensical.org/docs/setup/navigation/) key in its configuration file.

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

The only exception to that rule is links that many pages may need to reference. Such links may be added to [this file](https://github.com/DataDog/datadog-agent/blob/main/docs/public/.snippets/links.txt) that all pages are able to reference.

### Linking to source code

{% raw %}
Links to files or directories in this repository must use the `<<<SRC>>>` variable as the base URL:

```markdown
The [aggregator](<<<SRC>>>/pkg/aggregator) turns samples into series, see
[`time_sampler.go`](<<<SRC>>>/pkg/aggregator/time_sampler.go).
```
{% endraw %}

The variable renders as `https://github.com/DataDog/datadog-agent/blob/<ref>` where `<ref>` is the pull request branch during validation and `main` on the deployed sites. This allows a pull request that adds, moves, or renames source code to link to it from documentation without waiting for a merge.

During `dda run docs build --check`, every such link (including plain `blob/main` links) is resolved against the local checkout instead of GitHub, which both validates links to directories and catches links that a rename in the same pull request would break.

Do not append line-number fragments such as `#L42` to source links: they cannot be validated and silently rot as the file changes. Link to the file and name the function or type in the surrounding text instead.

## Abbreviations

[Abbreviations](https://squidfunk.github.io/mkdocs-material/reference/tooltips/#adding-abbreviations) like DSD may be added to [this file](https://github.com/DataDog/datadog-agent/blob/main/docs/public/.snippets/abbrs.txt) which will make it so that a tooltip will be displayed on hover.
