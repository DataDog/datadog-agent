# spEARS Agent Rules

This document provides **strict workflow rules** for LLM agents implementing the
spEARS methodology. For full methodology details, see [SPEARS.md](./SPEARS.md).

## Quick Reference

```text
specs/feature-name/
├── requirements.md   # WHAT (EARS format, immutable IDs, NO status)
├── design.md         # HOW (architecture, must trace to REQ-*)
└── executive.md      # STATUS (temporal link, NO code blocks)
```

### The Temporal Model

- **requirements.md**: Timeless. Unimplemented requirements, defined as status
  with a '❌', are valid scope - just not built yet.
- **design.md**: Slightly ahead of reality. Describes HOW to build requirements.
  All content must trace to a REQ-*.
- **executive.md**: The temporal link. ONLY document that must reflect current
  reality.

## When to Create Specs

**CREATE specs when:**

- Feature involves multiple components
- Clear acceptance criteria needed
- Complex logic requiring systematic testing
- Feature will evolve over time

**SKIP specs for:**

- Trivial bug fixes
- Simple refactorings
- Documentation-only changes
- Minor adjustments without new functionality

---

## Workflow: Creating a New Feature Spec

### Step 1: Create Directory

```bash
mkdir -p specs/feature-name
```

### Step 2: Write requirements.md

**Template:**

```markdown
# [Feature Name]

## User Story

As a [user type], I need to [capability] so that [benefit].

## Requirements

### REQ-[ABBREV]-001: [User Benefit Title]

WHEN [trigger condition]
THE SYSTEM SHALL [expected behavior]

WHEN [edge case]
THE SYSTEM SHALL [error handling]

**Rationale:** [Why does the USER care?]

---
```

**CRITICAL RULES:**

- NO status fields (status lives in executive.md)
- NO implementation details (implementation lives in design.md)
- NO "Updated YYYY-MM-DD" notes (git tracks history)
- IDs are IMMUTABLE once assigned

### Step 3: Write design.md

**CRITICAL:** design.md is written BEFORE implementation, not after.

Document technical design decisions:

- Architecture overview
- Data models (language-agnostic where possible)
- API contracts (if applicable)
- Error handling strategy
- Key technical decisions and trade-offs

**PURPOSE:** design.md is where the team discusses approaches and reaches
consensus BEFORE writing code. Implementation follows design, not the other way
around.

**RULE:** Get team agreement on design.md before proceeding to implementation.

### Step 4: Write executive.md

**Template:**

```markdown
# [Feature Name] - Executive Summary

## Requirements Summary

[250 words max, user-focused]

## Technical Summary

[250 words max, architecture-focused]

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-XX-001:** [Title] | ❌ Not Started | - |

**Progress:** 0 of N complete
```

**STRICT RULES:**

- 250 words max per summary
- ZERO code blocks (no exceptions)
- NO fluff ("tests run on every PR", etc.)
- Include requirement titles in table

**Inline backticks ARE allowed** for technical terms (`config.yaml`), file paths
(`src/auth.ts`), and requirement IDs (`REQ-XX-001`). Only code blocks are
prohibited.

---

## Workflow: Implementing Requirements

**The correct order: Requirements → Design → Tests → Implementation**

### Step 1: Ensure Requirements Are Complete

- requirements.md exists with EARS-formatted requirements
- Get feedback from team, iterate until requirements are clear
- Requirements should be crystal clear before proceeding

### Step 2: Complete design.md FIRST

**CRITICAL:** Design comes BEFORE code.

- Document architecture decisions
- Specify data models, API contracts, component interactions
- Discuss trade-offs with team
- Get agreement on technical approach
- **All content must trace to a REQ-* in requirements.md**

**WHY:** Clear requirements + agreed design makes TDD powerful. You know exactly
what to test because the expected behavior is unambiguous.

### Step 3: Update Status

In `executive.md`, change status: ❌ → 🔄

### Step 4: Write Tests

TDD is recommended but not strictly required. Clear requirements and design make
tests easier to write:

```text
// @requirement REQ-XX-001
// Test that [expected behavior from requirements]
```

### Step 5: Implement with Requirement Comments

Link code to requirements:

```text
// REQ-XX-001: [Brief description]
// Implementation of [requirement title]
```

### Step 6: Update design.md If Needed

As implementation reveals new insights:

- Update design.md to reflect actual implementation
- Document deviations from original design and why
- Keep design.md as accurate living documentation

### Step 7: Update Status

In `executive.md`, change status: 🔄 → ✅

---

## Workflow: Modifying Requirements

1. **NEVER change requirement IDs**
2. Update EARS statements in requirements.md (git shows history)
3. Update design.md implementation section
4. Update affected tests
5. Update executive.md status

### Adding New Requirements

- Use next sequential ID (REQ-XX-002, REQ-XX-003, etc.)
- NEVER reuse or renumber existing IDs

### Deprecating Requirements

```markdown
### REQ-XX-003: [Original Title]

**DEPRECATED:** Replaced by REQ-XX-008

[Original EARS statements preserved]

**Deprecation Reason:** [Why this was replaced]
```

---

## Requirements Quality Checklist

**RUN THIS CHECKLIST on every requirement before committing.**

### User-Centricity Check

- [ ] Title describes USER BENEFIT, not system feature
- [ ] WHEN clause describes user action/context, not system internals
- [ ] SHALL clause describes observable user outcome
- [ ] Rationale answers "why does the user care?" OR "does this provide value to
  the user?"
- [ ] Non-technical user could understand the value

**Core principle:** spEARS projects emphasize incremental user-facing value over
technical concerns.

### Implementation-Creep Check

- [ ] No data structure field names (geohash, latitude, timestamp)
- [ ] No algorithm/technology names (Redis, JWT, HTTP endpoint)
- [ ] No "HOW" details (belongs in design.md)
- [ ] No code-like language or jargon

### Testability Check

- [ ] Observable behavior verifiable without reading code
- [ ] Specific criteria (numbers, states, messages)
- [ ] Clear success/failure conditions

### Self-Containment Check (CRITICAL - ALL DOCUMENTS)

**This is the most violated principle.** All spec documents MUST be
understandable without external context.

**Severity by document:**

1. **design.md** - MOST CRITICAL. Free-form prose invites vague references. This
   is where violations happen most.
2. **executive.md** - HIGH. Summaries often reference "improvements" without
   specifying what improved.
3. **requirements.md** - MODERATE. EARS structure naturally guards against this,
   but rationales can still violate.

**Checklist:**

- [ ] No time-dependent references (see banned phrases)
- [ ] No comparative language requiring prior knowledge
- [ ] Document fully understandable standalone
- [ ] Both positive and negative cases explicitly stated
- [ ] No references to other documents for behavior definition

**BANNED PHRASES (rewrite immediately):**

- "as before" / "as currently implemented" / "previously"
- "maintain existing behavior" / "continue to work as expected"
- "as it does today" / "unchanged from current behavior"
- "same as [other feature]" / "like the login flow"
- "following the established pattern" / "the usual error handling"
- "standard validation rules" / "default timeout values"

---

#### design.md Self-Containment Anti-Patterns

**Implicit Versioning** - References "new/updated/improved" without specifying
what:

| BAD | GOOD |
|-----|------|
| "Use the new validation logic" | "Validate email addresses using RFC 5322 regex pattern; reject addresses without @ symbol or with consecutive dots" |
| "Apply updated rate limits" | "Rate limit: 100 requests per minute per API key, returning HTTP 429 with Retry-After header when exceeded" |
| "The improved caching strategy" | "Cache user profile data with 15-minute TTL; invalidate on any profile update" |
| "Modern authentication flow" | "Authentication via OAuth 2.0 Authorization Code flow with PKCE; access tokens expire after 1 hour, refresh tokens after 30 days" |
| "Use revised error messages" | "Validation errors return JSON with 'field', 'code', and 'message' keys; message must be user-facing English text under 100 characters" |
| "The refactored payment module" | "Payment processing: validate card via Stripe API, create pending transaction record, then capture payment; rollback transaction on capture failure" |

**Relative Comparisons** - Compares to unstated baseline:

| BAD | GOOD |
|-----|------|
| "Faster than the current implementation" | "Search queries must return within 200ms at p95 for datasets under 1M records" |
| "More secure than what we have" | "Passwords: minimum 12 characters, at least one uppercase, one lowercase, one number; bcrypt hashing with cost factor 12" |
| "Simpler than the existing approach" | "Configuration via single YAML file at `/etc/app/config.yml` with schema validation on startup" |
| "Fewer API calls than the old version" | "Dashboard load requires maximum 3 API calls: user profile, permissions, and recent activity" |
| "Better error handling than before" | "All database operations wrapped in try/catch; connection failures trigger 3 retries with exponential backoff (1s, 2s, 4s) before surfacing error to user" |
| "More flexible than the legacy system" | "Supports export to CSV, JSON, and Excel formats; user selects format via dropdown, default is CSV" |

---

**WHY:** If someone reads any spec document in 2 years, they should understand
it completely without reading git history, other docs, or the codebase.

---

## Red Flags (Rewrite Immediately)

- "The system SHALL return/include/store/cache..." (implementation language)
- Technical acronyms without user context (WGS84, JWT, HTTP 429)
- Requirement title ends in "-ing" (processing, caching, querying)
- Rationale mentions database, cache, algorithm, or data structure
- Time-dependent references: "as before", "previously", "maintain existing"

## Green Flags (Good Signs)

- Title starts with user action verb (Discover, Show, Enable, View)
- WHEN clause starts with "When a user..." or "When exploring..."
- SHALL clause describes what user sees/experiences
- Rationale uses: curiosity, discover, explore, understand, trust

---

## Anti-Patterns to Avoid

### Implementation Leak (MOST COMMON PROBLEM)

This is the anti-pattern that causes the most real-world pain.

**Bad: Technology/infrastructure in requirements**

```markdown
❌ THE SYSTEM SHALL use Redis for caching
❌ THE SYSTEM SHALL query the PostgreSQL database
❌ THE SYSTEM SHALL call the /api/v1/users endpoint
❌ THE SYSTEM SHALL store data in the user_sessions table
❌ THE SYSTEM SHALL use JWT tokens for authentication
❌ THE SYSTEM SHALL implement rate limiting using sliding window algorithm
```

**Good: Behavior without implementation**

```markdown
✅ THE SYSTEM SHALL persist cache across server restarts
✅ THE SYSTEM SHALL retrieve user data within 200ms
✅ THE SYSTEM SHALL maintain session across browser refreshes
✅ THE SYSTEM SHALL authenticate users securely
✅ THE SYSTEM SHALL limit requests to prevent abuse
```

**Bad: Data structure field names in requirements**

```markdown
❌ THE SYSTEM SHALL return user_id, created_at, and status fields
❌ THE SYSTEM SHALL include the geohash identifier
❌ WHEN the is_active flag is true
❌ THE SYSTEM SHALL set retry_count to 3
❌ THE SYSTEM SHALL populate the metadata object
```

**Good: User-visible information**

```markdown
✅ THE SYSTEM SHALL show the user's unique identifier, account creation date, and current status
✅ THE SYSTEM SHALL display a location reference
✅ WHEN the user's account is active
✅ THE SYSTEM SHALL retry failed operations up to 3 times
✅ THE SYSTEM SHALL include additional context about the item
```

**Bad: Algorithm in requirements**

```markdown
❌ WHEN a viewport query is received
THE SYSTEM SHALL complete within 500ms using geohash prefix queries
```

**Good: Performance target without algorithm**

```markdown
✅ WHEN user explores a region by panning or zooming
THE SYSTEM SHALL update displayed activity within 500ms
```

### Orphaned Content in design.md (MOST CRITICAL)

Content in design.md that cannot trace to a REQ-* in requirements.md is a
violation. This is how specs become roadmaps for undefined work.

**Bad: Future considerations without requirements**

```markdown
❌ ## Future Considerations
We could later add support for batch processing...

❌ ## Phase 2 Enhancements
In the next iteration, we'll implement caching...

❌ ## Extensibility
The architecture supports future OAuth providers...
```

**Good: All content traces to requirements**

```markdown
✅ ## REQ-RL-001 Implementation
Rate limiting uses sliding window algorithm...

✅ ## REQ-RL-002 Implementation
Quota display shows remaining requests...

✅ ## Architecture Overview
Components implementing REQ-RL-001 through REQ-RL-003...
```

**The test:** Can you annotate every section of design.md with the REQ-* it
supports? If not, that content belongs in an issue tracker.

### Wrong Document Location (ENFORCE STRICTLY)

**This is a core principle. Violations corrupt the entire system.**

**NEVER: Status in requirements.md**

```markdown
❌ ### REQ-XX-001: Feature Name
**Status:** In Progress
**Implemented:** 2025-01-15
**Test Coverage:** 80%
```

Status belongs ONLY in executive.md.

**NEVER: Code blocks in executive.md**

```markdown
❌ ## Technical Summary
Rate limiting enforces request quotas:
\`\`\`rust
pub fn check(&self, ip: &str) -> Result<()>
\`\`\`
```

ZERO code blocks in executive.md. No exceptions. Not even one-liners.

Note: Inline backticks ARE allowed. This is fine:

```markdown
✅ ## Technical Summary
Rate limiting uses `RateLimiter` to enforce per-IP quotas. Configuration in
`config/limits.yaml`. See `src/middleware/rate_limit.rs` for implementation.
```

**NEVER: Implementation details in requirements.md**

```markdown
❌ ### REQ-XX-001: Cache User Data
THE SYSTEM SHALL use Redis with 1-hour TTL
**Implementation:** See src/cache/redis.rs
```

Implementation details belong in design.md.

**NEVER: Requirement definitions in design.md**

```markdown
❌ ## Requirements
### REQ-XX-001: Feature Name
WHEN user clicks button...
```

Requirements belong ONLY in requirements.md.

### Bad Requirement Titles (CRITICAL)

Titles set the tone. Bad titles poison the whole requirement.

**Bad: Implementation-focused titles**

```markdown
❌ REQ-XX-001: Database Query Optimization
❌ REQ-XX-002: Redis Cache Integration
❌ REQ-XX-003: API Endpoint Implementation
❌ REQ-XX-004: Middleware Processing
❌ REQ-XX-005: Event Queue Handling
❌ REQ-XX-006: JWT Token Validation
```

**Good: User-benefit titles**

```markdown
✅ REQ-XX-001: Fast Data Retrieval
✅ REQ-XX-002: Instant Response for Repeat Visits
✅ REQ-XX-003: Access Account Information
✅ REQ-XX-004: Secure Request Handling
✅ REQ-XX-005: Reliable Message Delivery
✅ REQ-XX-006: Secure User Sessions
```

---

## Git Workflow

### Commit Messages

Reference requirement IDs:

```text
Implement rate limiting (REQ-RL-001)

- Add IP-based rate limiting middleware
- Return 429 with rate limit headers
- Add test coverage

Implements: REQ-RL-001, REQ-RL-002
```

### Traceability Verification

Before PR, verify grep-ability:

```bash
# All references to requirement
rg "REQ-RL-001"

# All requirements in feature
rg "^### REQ-" specs/feature-name/requirements.md

# All requirement comments in code
rg "// REQ-" src/
```

---

## Status Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Complete |
| 🔄 | In Progress |
| ⏭️ | Planned |
| ❌ | Not Started |
| ⚠️ | Manual verification only |
| 🟡 | Functional with gaps |
| N/A | Not applicable |

---

## DO / DON'T Summary

### DO

- Write requirements BEFORE implementation
- Use specific, measurable EARS criteria
- Link every requirement to tests
- Keep IDs immutable
- Update executive.md as work progresses
- Use git history for evolution tracking
- Keep executive.md concise (no code blocks)

### DON'T

- Write vague requirements ("fast", "good UX")
- Reuse requirement IDs
- Add status to requirements.md
- Include content in design.md without a corresponding REQ-*
- Create specs for trivial changes
- Let specs become stale
- Include code blocks in executive.md
- Add fluff to summaries

### Unimplemented vs Undefined (Critical Distinction)

**Unimplemented requirements** = REQ-* entries in requirements.md with ❌ status
in executive.md. These are legitimate scope, just not built yet. Design.md CAN
and SHOULD describe how to build them.

**Undefined features** = Content describing capabilities without a corresponding
REQ-* anywhere. This is scope creep. It belongs in an issue tracker, not the
spec. This includes "Future Considerations" sections, "Phase 2" plans, or
speculative extensibility.

---

## Reference

For complete methodology details, examples, and FAQ, see
[SPEARS.md](./SPEARS.md).

---

*spEARS: Simple Project with Easy Approach to Requirements Syntax*
