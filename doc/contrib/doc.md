# Writing developer docs

-----

Use these guidelines when adding or updating developer documentation in <<<repo("doc")>>>. The [documentation platform reference](../dev/tool/doc.md) describes the build commands, site configuration, and theme customizations.

## Organization

Organize navigation around subjects and reader goals. Keep each topic's explanations, tutorials, how-to guides, guidelines, and reference material together so readers can find the complete collection from one entry point. Define the navigation in the `nav` key in <<<repo("mkdocs.yml")>>>.

### Collections

Choose the collection by the question a page answers:

| Collection | Purpose | Directory |
| --- | --- | --- |
| [Home](../index.md) | Orient readers through getting started, a topic directory, and learning paths that link to canonical material throughout the site. | `doc/` |
| [Architecture](../arch/index.md) | Explain the Agent's runtime responsibilities, boundaries, data and control flows, and behavior across environments and configurations so readers can reason about the effects of changes. | `doc/arch/` |
| [Implementation](../impl/index.md) | Explain how to implement and extend Agent functionality through its frameworks, interfaces, conventions, and subsystems. | `doc/impl/` |
| [Development](../dev/index.md) | Explain how to set up, build, test, debug, and maintain the tools and infrastructure used for development. | `doc/dev/` |
| [Contributing](index.md) | State repository-wide requirements and processes for proposing, preparing, reviewing, and accepting changes. | `doc/contrib/` |

Classify content by the question it answers rather than its level of detail. For example, configuration resolution and propagation belong in Architecture; declaring a setting and looking up schema keywords belong together in Implementation.

Keep topic-specific requirements with their topic. Contributing can state a repository-wide validation policy and link to Development for instructions on producing the required evidence.

### Document types

Choose the page's form after identifying its topic and reader need. Use the [Diataxis documentation framework](https://diataxis.fr) to distinguish how individual pages serve readers within a topic.

| Type | Purpose |
| --- | --- |
| Explanation | Build understanding of runtime behavior or an implementation concept in the appropriate collection. |
| Tutorial | Lead a learner through an exercise with prerequisites, an expected outcome, and next steps. |
| How-to guide | Provide a focused procedure for a reader with a specific goal. |
| Guideline | State requirements or conventions at their applicable scope. |
| Reference | Provide precise technical descriptions of interfaces, schemas, implementation facilities, and tooling. |

In architectural explanations, include technical detail when it helps readers predict behavior or understand the effects of a change. State the conditions under which an explanation applies and link to its implementation sources. Keep exhaustive catalogs and generated details in the appropriate topic's reference material and link to them when relevant.

### Topic entry points

Give each substantial topic a shareable landing page that explains its scope and prerequisites, provides a recommended reading path, and links to common tasks, applicable conventions, and reference material. Readers should be able to start there without a maintainer supplying additional links or reading instructions.

Group pages by reader need as a topic grows. Add sections when useful content exists; avoid empty hierarchies.

Use the [topic directory](../topic.md) and links between topic overviews to connect subjects that span collections. Label these links by the question they answer, such as runtime configuration behavior versus schema development. Maintain one canonical account of each behavior, procedure, or interface; learning paths and alternative indexes should link to it.

### Independent navigation roots

Architecture, Implementation, and Development are peer navigation roots. Entering a root shows that collection's subject tabs and sidebars. Home and Contributing use ordinary sidebars.

Expand existing collections with topics and nested sidebar sections before adding another root. A new root should serve a substantial collection that needs its own tabs, with a landing page and a clear return route to the main docs. Nested roots are not supported. See the [navigation roots reference](../dev/tool/doc.md#navigation-roots) for configuration and behavior.

### Directories and URLs

Make tabs and sidebar sections correspond to directories so the filesystem generally follows the site navigation. The first tab of the main site or a navigation root is the exception: keep its pages directly in the containing directory. Use `index.md` for a landing page.

Keep directory names and Markdown filenames short yet meaningful. Use lowercase names with hyphens instead of underscores, and prefer singular words: `test.md` rather than `tests.md`, or `optional-dependency.md` rather than `optional_dependencies.md`. Let the containing directory supply context instead of repeating it in every filename. For example, the component testing tutorial lives at `impl/framework/component/tutorial/test.md`.

These rules govern source paths and the URLs they produce. Page titles and navigation labels can use fuller descriptions and plural words when appropriate.

When moving a page, update its navigation entry, incoming links, assets, and ownership paths.

## Ordered lists

Start each item in an [ordered list](https://spec.commonmark.org/0.31.2/#ordered-list-marker) with `1.` and let rendering supply the numbering. This avoids renumbering unrelated items when the list changes.

## Line continuations

Keep each prose paragraph on one source line and let the editor wrap it for display.

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

Use [admonitions](https://squidfunk.github.io/mkdocs-material/reference/admonitions/) for callouts. Reserve bold and italic formatting for short spans within sentences.

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

For links shared by many pages, add a reference definition to <<<repo("doc/.snippets/links.txt")>>>, which is included on every page.

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

The macro rejects Markdown pages under `doc`, which are linked relatively so that readers stay on this site.

/// warning
Macros are rendered everywhere on a page, including inside fenced code blocks. To show a macro rather than call it, wrap it in `<<<"<<% raw %>>">>>` and `<<<"<<% endraw %>>">>>` on the same line, the way the example above does. Putting those tags on their own lines leaves blank lines behind.

Macros do not run inside <<<repo("doc/.snippets")>>>, whose files are appended to each page after macros have already been rendered.
///

## Abbreviations

Add shared [abbreviations](https://squidfunk.github.io/mkdocs-material/reference/tooltips/#adding-abbreviations), such as DSD, to <<<repo("doc/.snippets/abbrs.txt")>>> so readers can see their definitions in tooltips.

## Preview and validation

Preview changes with `dda run docs serve`. Before submitting documentation changes, run `dda run docs build --check` to build the site and check its links. Inspect the affected pages and navigation in the preview.
