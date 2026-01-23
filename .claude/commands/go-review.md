---
description: Review Go code with all 7 specialized agents running in parallel (visible in UI)
---

Review the specified Go files or directory using all 7 specialized Go code reviewer agents running in parallel. You'll see each agent as a separate task in the UI.

Launch all 7 agents NOW using the Task tool with these subagent types:
1. go-naming-reviewer
2. go-errors-reviewer
3. go-testing-reviewer
4. go-concurrency-reviewer
5. go-performance-reviewer
6. go-structure-reviewer
7. go-style-reviewer

For each agent, provide this prompt:
```
Review all Go code in [TARGET]. For EACH issue you find, you MUST provide:
1. Exact file:line reference
2. Quote the problematic code
3. Detailed explanation of WHY it's a problem (with technical details)
4. The specific fix with code examples
5. The measurable benefit of fixing it

Follow your output format requirements exactly. Do NOT provide vague feedback.
```

Where [TARGET] is the path/files the user specified (or ask if not clear).

After launching all agents, tell the user they can see each agent running separately in the UI and will get 7 detailed reports.
