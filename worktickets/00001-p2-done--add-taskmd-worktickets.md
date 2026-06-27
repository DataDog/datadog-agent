# Add taskmd work tickets to datadog-agent

## Goal

Introduce `taskmd` as a lightweight work-ticket system for the Datadog Agent repository.

The initial repository convention will be:

- use a top-level `worktickets/` directory for taskmd tickets
- include the standard taskmd `_TEMPLATE.md` in that directory
- make the setup discoverable enough that contributors can run `taskmd --help` and start using the workflow

## Proposed initial changes

1. Create a top-level `worktickets/` directory.
2. Add `worktickets/_TEMPLATE.md` using the standard taskmd template.
3. Optionally add a short `worktickets/README.md` or repository-facing note if needed after reviewing how `taskmd` expects projects to document usage.
4. Verify the resulting layout with the local `taskmd` CLI, starting with `taskmd --help` and any relevant validation/list commands.

## Open question for follow-up

We will discuss and refine the exact `_TEMPLATE.md` contents before committing the final template.

## Validation

- Confirm `taskmd --help` is available in the workspace.
- Confirm `taskmd` recognizes or is compatible with the proposed `worktickets/` folder layout.
- Confirm the template filename and structure match taskmd expectations.
