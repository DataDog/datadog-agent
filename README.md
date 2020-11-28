# Datadog Agent (Deliveroo Edition)

This is a light fork of the DataDog Agent to track local customisations to the
agent. Wherever possible, changes should be kept to a minumum until upstream PRs
have been merged.

You can also read the original [README.md](README.original).

## Differences from upstream

- CI builds and packaging: Deliveroo uses CircleCI and only requires the RPM package.
- Dogstatsd Tag Filtering: Adapted from PR6526, to remove tags detected by the host
    agent that are not intended to be used for metric tagging. e.g. AWS ARNs
