# Context Window Reflection

**CRITICAL: Do NOT make any tool calls.** Use extended thinking to reflect
deeply on the current conversation context before responding.

## Your Task

Reflect on this work session and produce a **continuation prompt** that
captures:

1. **Progress Made**: Where are we relative to any phased plan (phase 1/2/3) or
   roadmap discussed? If no explicit phases exist, summarize loose progress
   against requirements.

2. **Current State**: What was actively being worked on when context is ending?

3. **Next Steps**: What should be picked up next? Be specific about which
   requirements or tasks remain.

4. **Worth Following Up On**: Capture anything you noticed during this session
   that deserves attention:
   - Failing tests encountered
   - Dead code or technical debt spotted
   - Inconsistencies in the codebase
   - Unresolved questions or decisions
   - Potential issues that weren't the focus but were observed

## Guidelines

- **Trust `specs/**/executive.md`** as the temporal link - it reflects current
  reality and where each spec is in its development journey
- **Trust `specs/**/*.md`** as the authoritative source of truth over all other
  documentation
- Reference **spEARS requirements** (EARS IDs) as the primary unit of work when
  applicable
- Keep the continuation prompt **minimal on context** - important info is
  already written to markdown documents in the repo
- Focus on **key insights specific to this session**, not general project
  background
- The prompt should clearly lay out the development vision and help the next
  context pick up seamlessly

## Output Format

After your reflection, output the continuation prompt in a fenced code block:

```text
<your continuation prompt here>
```

The continuation prompt should be immediately usable to resume work in a fresh
Claude Code context. The user will copy it manually.
