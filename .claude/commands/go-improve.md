---
name: go-improve
description: Review Go code and iteratively improve it until it achieves a perfect 10/10 score
---

You are tasked with improving Go code through an iterative feedback loop. Follow these steps:

1. **Initial Review**: Run a comprehensive Go code review on the provided path using the go-code-reviewer agent. The review should follow all Go best practices and provide a score out of 10.

2. **Iterative Improvement Loop**:
   - Analyze the critical, major, and minor issues identified in the review
   - Make targeted improvements to address the highest priority issues first
   - After making changes, run another comprehensive review
   - **IMPORTANT**: Do NOT stop until achieving a perfect 10/10 score - continue iterating no matter how many rounds it takes

3. **Improvement Strategy**:
   - Address critical issues immediately (correctness, bugs, data races)
   - Then tackle major issues (naming, error handling, test quality)
   - Finally polish minor issues (style, documentation, optimization)
   - Make surgical changes - don't rewrite unnecessarily
   - Preserve the original functionality and intent of the code
   - **When addressing reviewer concerns**: If you're confident about your implementation (e.g., a race condition cannot exist due to program structure), add explanatory comments in the code explaining your reasoning. This is good practice and helps the reviewer understand your design decisions.

4. **Completion**: Only stop when the review returns a perfect 10/10 score with no remaining issues. Do not give up or stop early.

5. **Summary**: When complete, provide a brief summary of:
   - What major improvements were made
   - How many review iterations were required
   - The final assessment score

Now proceed with reviewing and improving the code at the specified path.
