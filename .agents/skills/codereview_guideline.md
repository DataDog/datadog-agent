You are reviewing the SKILL.md files added or modified by this pull request,
against Anthropic's official skill-creator guidelines. Identify each changed
file under `.agents/skills/**/SKILL.md` from the PR diff, score it on the
dimensions below, and emit one finding per changed SKILL.md anchored to that
file's path and to the lines you are scoring.

## Dimension 1 — Description (triggering) / 25 pts
- Does it cover BOTH what the skill does AND when to trigger?
- Does it list concrete trigger terms a real user would type?
- Is it actively "pushy" — inviting use, not just describing?
- Is the scope narrow enough to avoid false triggers?
- Does it avoid being so vague that the agent undertriggers?
- Does the skill name reflect the skill's purpose?
- Does the scope overlap with an existing skill?

## Dimension 2 — Writing philosophy / 25 pts
- Does it explain WHY instructions matter, rather than just issuing commands?
- Does it avoid heavy-handed ALWAYS/NEVER/MUST in all-caps?
- Are instructions general enough to work across many prompts,
  not just the examples the author had in mind?
- Does it use imperative form ("Run X", not "X can be run")?
- Does it avoid over-narrow, example-specific rules that would
  cause the skill to overfit to particular inputs?
- Any high risk skill (any skill that interacts with production) must have
  safe guards documented to limit blast radius. Is it documented and are
  there user prompts that confirm changes before operating on production?
- Are embedded scripts easy to understand and free of cryptic behavior? Prefer
  python or go over shell when the script is longer than 10-20 lines.


## Dimension 3 — Structure and progressive disclosure / 25 pts
- Is the body under 500 lines?
- If > 300 lines, is there a table of contents or clear section headers?
- Are bundled resources (scripts/, references/, assets/) used for things
  every invocation would otherwise recreate from scratch?
- Are references annotated with WHEN to load them?
- Does the skill lead with the common path, edge cases later?
- Is the skill focused on a single purpose? Flag skills that do "too much"
  and should be split into smaller, composable skills.

## Dimension 4 — Output definition and examples / 25 pts
- Is the expected output format explicitly defined
  (with a template or example structure)?
- Are there concrete input/output examples, not just abstract descriptions?
- Are success criteria clear enough that two people would agree
  on whether the skill worked?
- Are dependencies or prerequisites stated?
- Does the skill declare an owning team? Every AI artifact must have at
  least one team owner.

## Output
For each changed SKILL.md, emit one finding whose body contains:
- Scores per dimension (out of 25 each, for a total out of 100)
- Top 3 actionable improvements grounded in the guidelines above
- A suggested description rewrite if dimension 1 scored < 18. In case of
  overlap suggest enhancements to the existing skill.
- Overall recommendation: Request Changes (<60) / Approve with suggestions (60–79) / Approve (≥80)

Set `priority` from the overall recommendation: 1 for Request Changes, 2 for
Approve with suggestions, 3 for Approve.
