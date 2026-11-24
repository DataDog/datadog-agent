# Learning paths

These paths connect existing guides for common goals. For environment setup and your first change, start with [Getting started](start.md).

## Implement a component

Start with the [component framework overview](impl/framework/component/index.md#learning-path). It provides the reading order for Fx concepts, component conventions, creation, testing, and integration into a binary. Keep the topic open to find its task guides and interface reference.

## Add a configuration setting

Start with the [configuration schema overview](impl/framework/config/index.md#learning-path). Follow its path through annotated examples, setting declarations, validation, and generation. Use the keyword and command references as needed.

## Validate a change

1. [Choose the test approach](dev/test/category.md) appropriate to the behavior you are changing.
1. [Run unit tests](dev/test/unit.md) and [static analysis](dev/test/static-analysis.md).
1. When the change needs infrastructure or deployment coverage, follow [end-to-end testing](dev/test/e2e/index.md) or [manual QA](dev/test/qa/index.md).
1. Use the [contribution checklist](contrib/index.md#pull-requests) to prepare the PR and its validation evidence.

## Improve documentation

1. Read the [organization and writing standards](contrib/doc.md).
1. Use the [documentation platform commands](dev/tool/doc.md#build-and-preview-commands) to preview the site and check its links.
1. Follow the [contribution process](contrib/index.md) to submit the change.
