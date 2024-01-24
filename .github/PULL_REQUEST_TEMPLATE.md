<!--
* New contributors are highly encouraged to read our
  [CONTRIBUTING](/CONTRIBUTING.md) documentation.
* The pull request:
  * Should only fix one issue or add one feature at a time.
  * Must update the test suite for the relevant functionality.
  * Should pass all status checks before being reviewed or merged.
* Commit titles should be prefixed with general area of pull request's change.

-->
### What does this PR do?

<!--
* A brief description of the change being made with this pull request.
* If the description here cannot be expressed in a succinct form, consider
  opening multiple pull requests instead of a single one.
-->

### Motivation

<!--
* What inspired you to submit this pull request?
* Link any related GitHub issues or PRs here.
-->

### Additional Notes

<!--
* Anything else we should know when reviewing?
* Include benchmarking information here whenever possible.
* Include info about alternatives that were considered and why the proposed
  version was chosen.
-->

### Possible Drawbacks / Trade-offs

<!--
* What are the possible side-effects or negative impacts of the code change?
-->

### Describe how to test/QA your changes

<!--
* Write here in detail or link to detailed instructions on how this change can
  be tested/QAd/validated, including any environment setup.
-->

### Reviewer's Checklist
<!--
* Authors can use this list as a reference to ensure that there are no problems
  during the review but the signing off is to be done by the reviewer(s).

Note: Adding GitHub labels is only possible for contributors with write access.
-->

- [ ] If known, an appropriate milestone has been selected; otherwise the `Triage` milestone is set.
- [ ] Use the `major_change` label if your change either has a major impact on the code base, is impacting multiple teams or is changing important well-established internals of the Agent. This label will be use during QA to make sure each team pay extra attention to the changed behavior. For any customer facing change use a releasenote.
- [ ] Changed code has automated tests for its functionality.
- [ ] Adequate QA/testing plan information is provided. Except if the `qa/skip-qa` label, with required either `qa/done` or `qa/no-code-change` labels, are applied.
- [ ] At least one `team/..` label has been applied, indicating the team(s) that should QA this change.
- [ ] If applicable, docs team has been notified or [an issue has been opened on the documentation repo](https://github.com/DataDog/documentation/issues/new).
- [ ] If applicable, the [config template](https://github.com/DataDog/datadog-agent/blob/main/pkg/config/config_template.yaml) has been updated.
