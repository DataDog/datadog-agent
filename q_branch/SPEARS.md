# spEARS: Simple Project with EARS

**spEARS** (Simple Project with Easy Approach to Requirements Syntax) is a
lightweight requirements-based methodology that provides explicit traceability
from business requirements → tests → code.

## Table of Contents

- [Overview](#overview)
- [Why spEARS?](#why-spears)
- [The Three-Document Pattern](#the-three-document-pattern)
- [EARS Format Guide](#ears-format-guide)
- [File Structure](#file-structure)
- [Workflow](#workflow)
- [Traceability](#traceability)
- [Examples](#examples)
- [FAQ](#faq)

## Overview

### Core Principles

1. **Requirements First**: Define what needs to be built before writing tests or
   code
2. **Testable Specifications**: Every requirement must be verifiable
3. **Immutable Traceability**: Requirements get permanent IDs that never change
4. **Living Documentation**: Specs evolve with the codebase, not as separate
   artifacts
5. **YAGNI/KISS**: Only use specs when they add clear value

### When to Create Specs

**Create specs when:**

- Building features with multiple components
- Implementing complex logic needing clear acceptance criteria
- Working on features that will evolve over time
- Starting projects with 3+ meaningful requirements

**Skip specs for:**

- Trivial bug fixes
- Simple refactorings without behavior change
- One-off scripts or utilities
- Projects fully understood by reading the README

## Why spEARS?

### Problems It Solves

**Problem 1: "Why did we build this?"**

- Without documented requirements, future maintainers don't understand intent
- Code comments describe "what" not "why"
- Git history provides implementation details, not business context

**Solution:** `requirements.md` captures business need and acceptance criteria

**Problem 2: "Is this feature complete?"**

- No clear definition of "done"
- Edge cases discovered in production
- Unclear verification status

**Solution:** EARS format provides verifiable acceptance criteria,
`executive.md` tracks status

**Problem 3: "What will break if I change this?"**

- Hard to find all code related to a feature
- Implementation doesn't clearly link to requirements
- Refactoring is risky without understanding dependencies

**Solution:** Requirement IDs create grep-able links between requirements,
tests, and code

**Problem 4: "What tests do I need to write?"**

- Easy to miss edge cases
- Tests written after code may just verify current behavior
- No systematic approach to test planning

**Solution:** EARS statements directly translate to test cases (one WHEN/SHALL =
one test)

## The Three-Document Pattern

Each feature gets a spec directory with three files:

```text
specs/feature-name/
├── requirements.md   # WHAT to build (EARS format, immutable IDs, timeless)
├── design.md         # HOW to build it (architecture, implementation, living)
└── executive.md      # WHERE are we (status tracking, executive summaries, authoritative)
```

### Separation of Concerns

| Document | Contains | Never Contains |
|----------|----------|----------------|
| **requirements.md** | EARS requirements, rationale, user stories | Status, implementation details, test coverage |
| **design.md** | Architecture, data models, API contracts, file locations | Status, requirement definitions, features without REQ-* |
| **executive.md** | Status table, summaries, verification coverage | Code blocks, detailed requirements |

### The Temporal Model

The three documents have different relationships with time:

- **requirements.md**: Timeless definitions. Requirements exist before
  implementation. An unimplemented requirement (❌ status) is still a valid,
  in-scope requirement - it's just not built yet.

- **design.md**: Can be slightly ahead of reality. Describes HOW to build
  requirements. May document the approach for not-yet-implemented requirements.
  **Critical rule:** All content must trace to a REQ-* in requirements.md.
  Content without a corresponding requirement is scope creep and belongs in an
  issue tracker.

- **executive.md**: The temporal link. The ONLY document that must reflect
  current reality. Tracks where the spec is in its development journey:
  - At 0%: "Nothing implemented, here's what's planned"
  - At 50%: "ABC complete, XYZ is next"
  - At 100%: A summary of the complete feature

## EARS Format Guide

EARS (Easy Approach to Requirements Syntax) was developed at Rolls-Royce for
aviation systems. It provides a simple, consistent structure for writing
unambiguous requirements.

### Basic Structure

```text
WHEN [trigger condition]
THE SYSTEM SHALL [expected behavior]
```

### The Five EARS Patterns

#### 1. Ubiquitous Requirements (Always true)

```markdown
THE SYSTEM SHALL validate email format before account creation
THE SYSTEM SHALL encrypt passwords using bcrypt
```

#### 2. Event-Driven Requirements (State changes)

```markdown
WHEN user clicks "Submit" button
THE SYSTEM SHALL validate form fields

WHEN API returns 500 error
THE SYSTEM SHALL retry request up to 3 times
```

#### 3. State-Driven Requirements (Conditional behavior)

```markdown
WHILE user is authenticated
THE SYSTEM SHALL display logout button

WHILE request queue is full
THE SYSTEM SHALL return 503 Service Unavailable
```

#### 4. Unwanted Behavior (Explicit prohibitions)

```markdown
IF user quota is exhausted
THE SYSTEM SHALL NOT process new requests

IF authentication token is invalid
THE SYSTEM SHALL NOT return sensitive data
```

#### 5. Optional Features (Configurable behavior)

```markdown
WHERE cache is enabled
THE SYSTEM SHALL return cached data within 100ms

WHERE premium mode is active
THE SYSTEM SHALL include extended analysis
```

### Writing Good EARS Requirements

#### Good Examples

**Specific and Measurable:**

```markdown
WHEN user makes 11th request from same IP within 1 hour
THE SYSTEM SHALL return HTTP 429 with X-RateLimit-Remaining: 0 header
```

**Error Conditions Explicit:**

```markdown
WHEN external API returns 503 error
THE SYSTEM SHALL display "Service temporarily unavailable" message
```

**Edge Cases Covered:**

```markdown
WHEN rate limit resets at hour boundary
THE SYSTEM SHALL allow new requests from previously blocked IPs
```

#### Bad Examples

**Vague:**

```markdown
❌ THE SYSTEM SHALL be fast
✅ THE SYSTEM SHALL respond within 2 seconds for cached data
```

**Ambiguous:**

```markdown
❌ THE SYSTEM SHALL handle errors gracefully
✅ WHEN database connection fails, THE SYSTEM SHALL display retry message
```

**Not Testable:**

```markdown
❌ THE SYSTEM SHALL provide good user experience
✅ WHEN form validation fails, THE SYSTEM SHALL highlight invalid fields
```

**Implementation Detail (belongs in design.md):**

```markdown
❌ THE SYSTEM SHALL use Redis for caching
✅ THE SYSTEM SHALL persist cache across server restarts
```

### Requirement ID Format

Use immutable IDs: `REQ-[ABBREV]-###`

- **[ABBREV]**: Short abbreviation (e.g., RL for Rate Limiting, UA for User
  Auth)
- **###**: Zero-padded sequential number (001, 002, etc.)
- **Once assigned, IDs are NEVER reused or changed**

Examples:

- `REQ-RL-001` - Rate Limiting requirement #1
- `REQ-UA-003` - User Auth requirement #3

### Requirement Titles

Titles SHALL describe USER BENEFITS, not system features.

**Good (User Benefit Focused):**

```markdown
✅ REQ-NM-001: Discover Recent Activity in a Region
✅ REQ-RL-001: Prevent Abuse Attacks
✅ REQ-CC-001: View Current Status Without Waiting
```

**Bad (Implementation Focused):**

```markdown
❌ REQ-NM-001: Viewport-Based Query
❌ REQ-RL-001: IP-Based Rate Limiting
❌ REQ-CC-001: Cache Data Layer
```

### Rationale Guidelines

Every rationale MUST answer one of these questions:

- **"Why does the USER care?"** - What problem does this solve for them?
- **"Does this provide value to the user?"** - What tangible benefit do they
  get?

These are two sides of the same coin. spEARS projects emphasize incremental
user-facing value over technical concerns.

**Good Rationale (User Value Focused):**

```markdown
✅ "Users want to see activity without waiting. Fast response enables
curiosity-driven browsing - users can quickly scan across regions.
Slow responses would discourage exploration."
```

**Bad Rationale (Technical Focused):**

```markdown
❌ "Enables spatial discovery of cached data. The 500ms target ensures
responsive interaction. WGS84 is the standard coordinate system."
```

## File Structure

```text
ProjectName/
├── specs/
│   ├── feature-one/
│   │   ├── requirements.md
│   │   ├── design.md
│   │   └── executive.md
│   └── feature-two/
│       ├── requirements.md
│       ├── design.md
│       └── executive.md
├── src/
└── README.md
```

Even simple projects should identify their initial feature and create a spec for
it. This maintains consistency and makes it easy to add more features later.

### Naming Conventions

Use **kebab-case** for directory names:

- `rate-limiting` (not `RateLimiting` or `rate_limiting`)
- `user-accounts` (not `userAccounts`)

Match names to user-facing concepts:

- ✅ `quota-visibility` - Clear business feature
- ⚠️ `redis-storage` - Implementation detail, not a feature

### requirements.md Template

```markdown
# [Feature Name]

## User Story

As a [user type], I need to [capability] so that [benefit].

## Requirements

### REQ-[ABBREV]-001: [User Benefit Title]

WHEN [condition]
THE SYSTEM SHALL [behavior]

WHEN [edge case]
THE SYSTEM SHALL [error handling]

**Rationale:** [Why does the USER care?]

**Dependencies:** REQ-[ABBREV]-002 (if applicable)

---

### REQ-[ABBREV]-002: [Next Requirement]

WHEN [condition]
THE SYSTEM SHALL [behavior]

**Rationale:** [User benefit explanation]

---
```

**Key Principles:**

- NO status fields (status lives in executive.md)
- NO implementation sections (implementation lives in design.md)
- Git history shows evolution (no "Updated YYYY-MM-DD" notes)
- Requirements can be added, modified, or deprecated (ID never changes)

### design.md Template

**Purpose:** design.md serves two critical roles:

1. **During planning:** Technical design document where architecture decisions
   are made and agreed upon BEFORE implementation begins. This is where the team
   discusses approaches, trade-offs, and reaches consensus.

2. **During/after implementation:** Living documentation that reflects the
   actual system. Updated as implementation reveals new insights or requirements
   evolve.

**Key principle:** Implementation should follow design, not the other way
around. Write design.md first, get team agreement, then implement. Update
design.md when reality diverges from plan.

**Traceability rule:** Every section in design.md must trace to a requirement in
requirements.md. If you find yourself writing about features or capabilities
that don't have a REQ-* identifier, that content belongs in an issue tracker,
not the spec. This prevents design.md from becoming a roadmap for undefined
work.

```markdown
# [Feature Name] - Technical Design

## Architecture Overview

[High-level description of how components interact]

## Data Models

[Structs, interfaces, schemas - language/framework agnostic where possible]

## API Contracts

[Endpoint specifications if applicable]

## Component Interactions

[Sequence diagrams, data flow descriptions]

## Error Handling Strategy

[How errors are detected, reported, recovered]

## Security Considerations

[Authentication, authorization, data validation]

## Performance Considerations

[Caching, optimization strategies]

## Implementation Notes

### REQ-[ABBREV]-001 Implementation
- Location: [file paths]
- Approach: [technical approach]
- Trade-offs: [decisions and why]
```

### executive.md Template

**Purpose:** Authoritative status tracking. Target persona: busy technical
leader who wants essential facts.

```markdown
# [Feature Name] - Executive Summary

## Requirements Summary

[250 words max, user-focused: What problem does this solve? What can users do?]

## Technical Summary

[250 words max, architecture-focused: How is it built? Key technical decisions?]

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-[ABBREV]-001:** [Title] | ✅ Complete | Verified via [method] |
| **REQ-[ABBREV]-002:** [Title] | 🔄 In Progress | [Component] done, [other] pending |
| **REQ-[ABBREV]-003:** [Title] | ❌ Not Started | Planned for next iteration |

**Progress:** X of Y complete
```

**Status Legend:**

- ✅ Complete
- 🔄 In Progress
- ⏭️ Planned
- ❌ Not Started
- ⚠️ Manual verification only
- N/A - Not applicable

**Key Principles:**

- 250 words max for each summary
- NO code blocks (zero tolerance for triple-backtick code)
- NO fluff
- Include requirement titles in table

**Code in executive.md - What's Allowed:**

| Allowed | Prohibited |
|---------|------------|
| Inline backticks for technical terms (`config.yaml`, `REQ-XX-001`) | Code blocks (triple backticks) |
| File paths (`src/auth/login.ts`) | Multi-line code examples |
| Environment variables (`$API_KEY`) | Function/method implementations |
| Configuration keys | Pseudocode or algorithms |

## Workflow

### 1. Planning a New Feature

**Input:** User story or business requirement

**Steps:**

1. Create spec directory: `mkdir -p specs/feature-name`
2. Write `requirements.md` with EARS-formatted requirements (no status)
3. Write `design.md` with architecture approach
4. Write `executive.md` with status table (all ❌ initially)

**Output:** Complete spec ready for implementation

### 2. Implementing a Feature

**Input:** Completed spec

**Steps:**

1. Update `executive.md` status (❌ → 🔄)
2. Write tests with requirement references:

   ```typescript
   /** @requirement REQ-RL-001 */
   test('should enforce rate limit', async () => {
     // Test implementation
   });
   ```

3. Implement code with requirement comments:

   ```rust
   // REQ-RL-001: Rate limiting implementation
   pub async fn check_rate_limit(...) -> Result<...> {
     // Implementation
   }
   ```

4. Update `design.md` with implementation details
5. Update `executive.md` status (🔄 → ✅)

**Output:** Implemented feature with complete traceability

### 3. Modifying an Existing Feature

**Steps:**

1. Review existing `requirements.md`
2. If new requirement needed: add with next sequential ID (NEVER reuse IDs)
3. If existing requirement changes: update EARS statements (git shows evolution)
   and iterate on `design.md`

4. Update implementation with requirement comments
5. Update `executive.md` status

### 4. Deprecating a Requirement

**Steps:**

1. Do NOT delete from `requirements.md`
2. Add deprecation note:

   ```markdown
   ### REQ-RL-003: [Title]

   **DEPRECATED:** Replaced by REQ-RL-008

   [Original EARS statements preserved]

   **Deprecation Reason:** [Why replaced]
   ```

3. Update `executive.md` status
4. Add deprecation comments to code

## Traceability

### Grep-Based Verification

The system is designed for **grep-based traceability**.

**Find all references to a requirement:**

```bash
rg "REQ-RL-001"
```

Expected output:

```text
specs/rate-limiting/requirements.md
22:### REQ-RL-001: Prevent Abuse Attacks

specs/rate-limiting/design.md
45:### REQ-RL-001 Implementation

specs/rate-limiting/executive.md
15:| **REQ-RL-001:** Prevent Abuse | ✅ Complete | ...

src/middleware/rate_limit.rs
45:// REQ-RL-001: Rate limiting implementation

tests/rate_limiting.spec.ts
25:/** @requirement REQ-RL-001 */
```

**Find all requirements in a feature:**

```bash
rg "^### REQ-" specs/rate-limiting/requirements.md
```

### Traceability Matrix

For each requirement:

```text
REQ-RL-001
  ├── requirements.md (definition)
  ├── design.md (implementation approach)
  ├── executive.md (status)
  ├── tests/ (@requirement tag in test files)
  ├── src/ (implementation comment in code)
  └── git log --grep="REQ-RL-001" (commits)
```

## Examples

### Minimal Example

**specs/build-info/requirements.md:**

```markdown
# Build Information Display

## User Story

As a user, I need to see which version is running so I can report issues accurately.

## Requirements

### REQ-BI-001: View Application Version

WHEN user views application footer
THE SYSTEM SHALL display 7-character git commit SHA

WHEN git SHA is not available
THE SYSTEM SHALL display "unknown"

**Rationale:** Users need version info to provide useful bug reports.
```

**specs/build-info/executive.md:**

```markdown
# Build Information - Executive Summary

## Requirements Summary

Users can view the application version in the footer to report issues accurately.

## Technical Summary

Footer displays `window.__GIT_SHA__` injected at build time. Falls back to "unknown".

## Status Summary

| Requirement | Status | Notes |
|-------------|--------|-------|
| **REQ-BI-001:** View Version | ✅ Complete | E2E test verifies footer |

**Progress:** 1 of 1 complete
```

## FAQ

### Q: Do I need specs for bug fixes?

**A:** Usually no. For simple bugs: fix, add regression test, reference issue in
commit. For bugs revealing missing requirements: add requirement, write test,
implement fix.

### Q: When do I create a new spec vs add to existing?

**A:** Create new when feature is logically independent. Add to existing when it
extends current capability or shares architecture.

### Q: What if requirements change frequently?

**A:** EARS handles change well. Add new requirements with new IDs. Update
existing statements (git shows history). Deprecate obsolete requirements (don't
delete). Immutable IDs provide stability.

### Q: Isn't this overhead?

**A:** Upfront cost, long-term savings. 15-30 minutes to write requirements
saves hours debugging unclear requirements, days refactoring untested code,
weeks onboarding developers.

### Q: How detailed should EARS statements be?

**A:** Detailed enough to implement clearly. If multiple interpretations
possible → too vague. If describes implementation → too detailed. Good test: Can
someone else implement from requirements alone?

### Q: Can requirements reference other requirements?

**A:** Yes, using Dependencies field:

```markdown
### REQ-RL-002: Quota Enforcement

**Dependencies:** REQ-RL-001 (requires rate limiting)
```

### Q: How do I handle configuration-dependent requirements?

**A:** Use EARS "WHERE" pattern:

```markdown
WHERE rate limiting is enabled
THE SYSTEM SHALL enforce 10 requests per hour limit

WHERE rate limiting is disabled
THE SYSTEM SHALL allow unlimited requests
```

## References

### External Resources

- [EARS Whitepaper (Rolls-Royce)](https://www.researchgate.net/publication/224079416_Easy_Approach_to_Requirements_Syntax_EARS)
- [Kiro Code Specs](https://kiro.dev/docs/specs/concepts/)
- [IEEE Guide to SRS](https://standards.ieee.org/standard/29148-2018.html)

### Related Files

- **SPEARS_AGENT.md** - LLM/agent workflow rules and checklists
- **specs/** - Feature specifications following this methodology

---

*spEARS: Simple Project with Easy Approach to Requirements Syntax*
