# CI Validation

Context for evaluating whether a Jira ticket is eligible based on CI information it contains.

**Read this when:** a ticket's description or comments contain links to GitLab CI logs and you need to understand the failure before deciding whether to attempt the ticket.

---

## Fetching CI Logs from a Ticket

If a Jira ticket links to a GitLab pipeline or job log, use the `/fetch-ci-results` skill from the [Claude Marketplace](https://github.com/DataDog/claude-marketplace-gpt) to read those logs:

```
/fetch-ci-results <GITLAB-URL>
```

Use the output to understand the failure context described in the ticket. This helps determine whether the fix is self-contained and implementable.

---

## Ignorable CI Checks

When evaluating a ticket that references CI failures, some checks are infrastructure-owned and not fixable by a code PR:

- `mergegate` — campaign authorization, not a code issue
- `validate-staging-merge` — staging branch validation
- Checks that only run on `main` or release branches

If the ticket's CI failure is only in ignorable checks, the ticket may not need a code fix — skip it.

---

## Flaky Tests

If the ticket was filed because of a CI failure, check `flakes.yaml` at the repo root to see if the test is a known flaky. If it is, the ticket may already be tracked and should be skipped.
