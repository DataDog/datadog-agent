# Datadog Agent CI Developer Experience Survey

**Purpose:** Understand developer experience with CI/CD pipeline to prioritize improvements

**Audience:** All datadog-agent contributors (last 6 months)

**Time to complete:** 5 minutes

**Responses:** Anonymous

---

## Section 1: CI Impact on Productivity

**Q1. How often do you wait for CI results before merging?**
- [ ] Every PR (100%)
- [ ] Most PRs (75-99%)
- [ ] Some PRs (25-74%)
- [ ] Rarely (1-24%)
- [ ] Never (0%)

**Q2. When CI is running, what do you typically do? (Select all that apply)**
- [ ] Wait for results before starting next task
- [ ] Context-switch to unrelated work
- [ ] Work on another PR
- [ ] Take a break
- [ ] Review others' PRs
- [ ] Other: _____________

**Q3. How often does slow CI negatively impact your productivity?**
- [ ] Daily
- [ ] 2-3 times per week
- [ ] Weekly
- [ ] 2-3 times per month
- [ ] Rarely or never

**Q4. Estimate average time you spend per week dealing with CI issues (flaky tests, re-runs, debugging failures):**
- [ ] 0-1 hours
- [ ] 1-3 hours
- [ ] 3-6 hours
- [ ] 6-10 hours
- [ ] More than 10 hours

---

## Section 2: Specific Pain Points

**Q5. Rate these CI problems by severity (1=Minor annoyance, 5=Major blocker):**

| Issue | 1 | 2 | 3 | 4 | 5 |
|-------|---|---|---|---|---|
| Flaky tests requiring re-runs | ☐ | ☐ | ☐ | ☐ | ☐ |
| Long pipeline duration (>30 min) | ☐ | ☐ | ☐ | ☐ | ☐ |
| Pipeline failures unrelated to my changes | ☐ | ☐ | ☐ | ☐ | ☐ |
| Difficulty understanding failure causes | ☐ | ☐ | ☐ | ☐ | ☐ |
| Jobs timing out | ☐ | ☐ | ☐ | ☐ | ☐ |
| Resource contention (jobs waiting for runners) | ☐ | ☐ | ☐ | ☐ | ☐ |
| Windows-specific test issues | ☐ | ☐ | ☐ | ☐ | ☐ |

**Q6. Which CI stages are most problematic for you? (Rank top 3)**

_Rank 1-3 where 1=Most problematic:_

- [ ] \_\_\_ Lint
- [ ] \_\_\_ Source tests (unit tests)
- [ ] \_\_\_ Binary builds
- [ ] \_\_\_ Package builds
- [ ] \_\_\_ E2E tests
- [ ] \_\_\_ Windows tests
- [ ] \_\_\_ Integration tests
- [ ] \_\_\_ Other: _____________

**Q7. What is your typical response when CI fails on a PR? (Select all that apply)**
- [ ] Investigate logs immediately
- [ ] Re-run failed jobs without investigation
- [ ] Ask in Slack for help
- [ ] Wait to see if it passes on retry
- [ ] Merge anyway if failure seems unrelated
- [ ] Give up and abandon the PR
- [ ] Other: _____________

---

## Section 3: Test Reliability

**Q8. How often do you encounter flaky tests on your PRs?**
- [ ] On every PR
- [ ] Most PRs (>75%)
- [ ] Frequently (25-75%)
- [ ] Occasionally (<25%)
- [ ] Rarely or never

**Q9. When you encounter a flaky test, what do you typically do?**
- [ ] Re-run the job until it passes
- [ ] Skip the test (t.Skip)
- [ ] Mark as flaky (flake.Mark)
- [ ] Debug and fix it
- [ ] Report it to the team
- [ ] Ignore it
- [ ] Other: _____________

**Q10. Do you trust CI results?**
- [ ] Yes, CI failures almost always indicate real issues with my code
- [ ] Mostly, but I need to verify failures aren't flaky
- [ ] Sometimes, requires significant investigation
- [ ] No, too many false positives
- [ ] Other: _____________

---

## Section 4: Developer Workflow

**Q11. How many times do you typically re-run CI jobs per PR?**
- [ ] 0 (passes first time)
- [ ] 1-2 times
- [ ] 3-5 times
- [ ] 6-10 times
- [ ] More than 10 times

**Q12. What would have the biggest impact on your productivity? (Rank top 3)**

_Rank 1-3 where 1=Highest impact:_

- [ ] \_\_\_ Faster overall pipeline duration
- [ ] \_\_\_ Fewer flaky tests
- [ ] \_\_\_ Better failure diagnostics/logs
- [ ] \_\_\_ More reliable Windows tests
- [ ] \_\_\_ Faster feedback on lint/unit tests
- [ ] \_\_\_ Reduced queue/wait times
- [ ] \_\_\_ Better CI documentation
- [ ] \_\_\_ Ability to run specific jobs locally
- [ ] \_\_\_ Other: _____________

**Q13. Do you run tests locally before pushing?**
- [ ] Always
- [ ] Usually
- [ ] Sometimes
- [ ] Rarely
- [ ] Never

**Q14. Why don't you run more tests locally? (Select all that apply)**
- [ ] I do run all tests locally
- [ ] Tests take too long locally
- [ ] My machine doesn't have enough resources
- [ ] Some tests only work in CI environment
- [ ] Setup is too complex
- [ ] I trust CI will catch issues
- [ ] Other: _____________

---

## Section 5: Open Feedback

**Q15. What specific CI jobs or tests are the most painful for you?**

_Free text:_

```
[Your answer here]
```

**Q16. If you could fix one thing about CI, what would it be?**

_Free text:_

```
[Your answer here]
```

**Q17. Any other feedback about the CI system?**

_Free text:_

```
[Your answer here]
```

---

## Optional: Demographics

**Q18. Which team do you primarily work on?**
- [ ] Agent Core
- [ ] Agent Platform
- [ ] APM
- [ ] Logs
- [ ] Security/CWS
- [ ] Network
- [ ] Containers
- [ ] Windows
- [ ] Other: _____________

**Q19. How long have you been contributing to datadog-agent?**
- [ ] < 3 months
- [ ] 3-6 months
- [ ] 6-12 months
- [ ] 1-2 years
- [ ] > 2 years

---

**Thank you for your feedback!**

Your responses will help prioritize CI improvements and directly impact developer experience.

Results will be shared with the team within 2 weeks.
