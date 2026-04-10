---
name: distill
description: "Extract an Allium specification from an existing codebase. Use when the user has existing code and wants to distil behaviour into a spec, reverse engineer a specification from implementation, generate a spec from code, turn implementation into a behavioural specification, or document what a codebase does in Allium terms."
---

# Distillation guide

This guide covers extracting Allium specifications from existing codebases. The core challenge is the same as forward elicitation: finding the right level of abstraction. In elicitation you filter out implementation ideas as they arise. In distillation you filter out implementation details that already exist. Both require the same judgement about what matters at the domain level.

Code tells you *how* something works. A specification captures *what* it does and *why* it matters. The skill is asking "why does the stakeholder care about this?" and "could this be different while still being the same system?"

## Scoping the distillation effort

Before diving into code, establish what you are trying to specify. Not every line of code deserves a place in the spec.

### Questions to ask first

1. **"What subset of this codebase are we specifying?"**
   Mono repos often contain multiple distinct systems. You may only need a spec for one service or domain. Clarify boundaries explicitly before starting.

2. **"Is there code we should deliberately exclude?"**
   - **Legacy code**: features kept for backwards compatibility but not part of the core system
   - **Incidental code**: supporting infrastructure that is not domain-level (logging, metrics, deployment)
   - **Deprecated paths**: code scheduled for removal
   - **Experimental features**: behind feature flags, not yet design decisions

3. **"Who owns this spec?"**
   Different teams may own different parts of a mono repo. Each team's spec should focus on their domain.

### The "Would we rebuild this?" test

For any code path you encounter, ask: "If we rebuilt this system from scratch, would this be in the requirements?"

- Yes: include in spec
- No, it is legacy: exclude
- No, it is infrastructure: exclude
- No, it is a workaround: exclude (but note the underlying need it addresses)

### Documenting scope decisions

At the top of a distilled spec, document what is included and excluded:

```
-- allium: 3
-- interview-scheduling.allium

-- Scope: Interview scheduling flow only
-- Includes: Candidacy, Interview, InterviewSlot, Invitation, Feedback
-- Excludes:
--   - User authentication (use auth library spec)
--   - Analytics/reporting (separate spec)
--   - Legacy V1 API (deprecated, not specified)
--   - Greenhouse sync (use greenhouse library spec)
```

The version marker (`-- allium: N`) must be the first line of every `.allium` file. Use the current language version number.

## Finding the right level of abstraction

Distillation and elicitation share the same fundamental challenge: choosing what to include. The tests below work in both directions, whether you are hearing a stakeholder describe a feature or reading code that implements it.

### The "Why" test

For every detail in the code, ask: "Why does the stakeholder care about this?"

| Code detail | Why? | Include? |
|-------------|------|----------|
| Invitation expires in 7 days | Affects candidate experience | Yes |
| Token is 32 bytes URL-safe | Security implementation | No |
| Sessions stored in Redis | Performance choice | No |
| Uses PostgreSQL JSONB | Database implementation | No |
| Slot status changes to 'proposed' | Affects what candidate sees | Yes |
| Email sent when invitation accepted | Communication requirement | Yes |

If you cannot articulate why a stakeholder would care, it is probably implementation.

### The "Could it be different?" test

Ask: "Could this be implemented differently while still being the same system?"

- If yes: probably implementation detail, abstract it away
- If no: probably domain-level, include it

| Detail | Could be different? | Include? |
|--------|---------------------|----------|
| `secrets.token_urlsafe(32)` | Yes, any secure token generation | No |
| 7-day invitation expiry | No, this is the design decision | Yes |
| PostgreSQL database | Yes, any database | No |
| "Pending, Confirmed, Completed" states | No, this is the workflow | Yes |

### The "Template vs Instance" test

Is this a **category** of thing, or a **specific instance**?

| Instance (often implementation) | Template (often domain-level) |
|--------------------------------|-------------------------------|
| Google OAuth | Authentication provider |
| Slack webhook | Notification channel |
| SendGrid API | Email delivery |
| `timedelta(hours=3)` | Confirmation deadline |

Sometimes the instance IS the domain concern. See "The concrete detail problem" below.

## The distillation mindset

### Code is over-specified

Every line of code makes decisions that might not matter at the domain level:

```python
# Code tells you:
def send_invitation(candidate_id: int, slot_ids: List[int]) -> Invitation:
    candidate = db.session.query(Candidate).get(candidate_id)
    slots = db.session.query(InterviewSlot).filter(
        InterviewSlot.id.in_(slot_ids),
        InterviewSlot.status == 'confirmed'
    ).all()

    invitation = Invitation(
        candidate_id=candidate_id,
        token=secrets.token_urlsafe(32),
        expires_at=datetime.utcnow() + timedelta(days=7),
        status='pending'
    )
    db.session.add(invitation)

    for slot in slots:
        slot.status = 'proposed'
        invitation.slots.append(slot)

    db.session.commit()

    send_email(
        to=candidate.email,
        template='interview_invitation',
        context={'invitation': invitation, 'slots': slots}
    )

    return invitation
```

```
-- Specification should say:
rule SendInvitation {
    when: SendInvitation(candidacy, slots)

    requires: slots.all(s => s.status = confirmed)

    ensures:
        for s in slots:
            s.status = proposed
    ensures: Invitation.created(
        candidacy: candidacy,
        slots: slots,
        expires_at: now + 7.days,
        status: pending
    )
    ensures: Email.created(
        to: candidacy.candidate.email,
        template: interview_invitation
    )
}
```

What we dropped:
- `candidate_id: int` became just `candidacy`
- `db.session.query(...)` became relationship traversal
- `secrets.token_urlsafe(32)` removed entirely (token is implementation)
- `datetime.utcnow() + timedelta(...)` became `now + 7.days`
- `db.session.add/commit` implied by `created`
- `invitation.slots.append(slot)` implied by relationship

### Ask "Would a product owner care?"

For every detail in the code, ask:

| Code detail | Product owner cares? | Include? |
|-------------|---------------------|----------|
| Invitation expires in 7 days | Yes, affects candidate experience | Yes |
| Token is 32 bytes URL-safe | No, security implementation | No |
| Uses SQLAlchemy ORM | No, persistence mechanism | No |
| Email template name | Maybe, if templates are design decisions | Maybe |
| Slot status changes to 'proposed' | Yes, affects what candidate sees | Yes |
| Database transaction commits | No, implementation detail | No |

### Distinguish means from ends

**Means:** how the code achieves something.
**Ends:** what outcome the system needs.

| Means (code) | Ends (spec) |
|--------------|-------------|
| `requests.post('https://slack.com/api/...')` | `Notification.created(channel: slack)` |
| `candidate.oauth_token = google.exchange(code)` | `Candidate authenticated` |
| `redis.setex(f'session:{id}', 86400, data)` | `Session.created(expires: 24.hours)` |
| `for slot in slots: slot.status = 'cancelled'` | `for s in slots: s.status = cancelled` |

## The concrete detail problem

The hardest judgement call: when is a concrete detail part of the domain vs just implementation?

### Google OAuth example

You find this code:
```python
OAUTH_PROVIDERS = {
    'google': GoogleOAuthProvider(client_id=..., client_secret=...),
}

def authenticate(provider: str, code: str) -> User:
    return OAUTH_PROVIDERS[provider].authenticate(code)
```

**Question:** Is "Google OAuth" domain-level or implementation?

**It is implementation if:**
- Google is just the auth mechanism chosen
- It could be replaced with any OAuth provider
- Users do not see or care which provider
- The code is written generically (provider is a parameter)

**It is domain-level if:**
- Users explicitly choose Google (vs Microsoft, etc.)
- "Sign in with Google" is a feature
- Google-specific scopes or permissions are used
- Multiple providers are supported as a feature

**How to tell:** Look at the UI and user flows. If users see "Sign in with Google" as a choice, it is domain-level. If they just see "Sign in" and Google happens to be behind it, it is implementation.

### Database choice example

You find PostgreSQL-specific code:
```python
from sqlalchemy.dialects.postgresql import JSONB, ARRAY

class Candidate(Base):
    skills = Column(ARRAY(String))
    metadata = Column(JSONB)
```

**Almost always implementation.** The spec should say:
```
entity Candidate {
    skills: Set<String>
    metadata: String?              -- or model specific fields
}
```

The specific database is rarely domain-level. Exception: if the system explicitly promises PostgreSQL compatibility or specific PostgreSQL features to users.

### Third-party integration example

You find Greenhouse ATS integration:
```python
class GreenhouseSync:
    def import_candidate(self, greenhouse_id: str) -> Candidate:
        data = self.client.get_candidate(greenhouse_id)
        return Candidate(
            name=data['name'],
            email=data['email'],
            greenhouse_id=greenhouse_id,
            source='greenhouse'
        )
```

**Could be either:**

**Implementation if:**
- Greenhouse is just where candidates happen to come from
- Could be swapped for Lever, Workable, etc.
- The integration is an implementation detail of "candidates are imported"

Spec:
```
external entity Candidate {
    name: String
    email: String
    source: CandidateSource
}
```

**Product-level if:**
- "Greenhouse integration" is a selling point
- Users configure their Greenhouse connection
- Greenhouse-specific features are exposed (like syncing feedback back)

Spec:
```
external entity Candidate {
    name: String
    email: String
    greenhouse_id: String?  -- explicitly modeled
}

rule SyncFromGreenhouse {
    when: GreenhouseWebhookReceived(candidate_data)
    ensures: Candidate.created(
        ...
        greenhouse_id: candidate_data.id
    )
}
```

### The "Multiple implementations" heuristic

Look for variation in the codebase:

- If there is only one OAuth provider, probably implementation
- If there are multiple OAuth providers, probably domain-level
- If there is only one notification channel, probably implementation
- If there are Slack AND email AND SMS, probably domain-level

The presence of multiple implementations suggests the variation itself is a domain concern.

## Distillation process

### Step 1: Map the territory

Before extracting any specification, understand the codebase structure:

1. **Identify entry points.** API routes, CLI commands, message handlers, scheduled jobs.
2. **Find the domain models.** Usually in `models/`, `entities/`, `domain/`.
3. **Locate business logic.** Services, use cases, handlers.
4. **Note external integrations.** What third parties does it talk to?

Create a rough map:
```
Entry points:
  - API: /api/candidates/*, /api/interviews/*, /api/invitations/*
  - Webhooks: /webhooks/greenhouse, /webhooks/calendar
  - Jobs: send_reminders, expire_invitations, sync_calendars

Models:
  - Candidate, Interview, InterviewSlot, Invitation, Feedback

Services:
  - SchedulingService, NotificationService, CalendarService

Integrations:
  - Google Calendar, Slack, Greenhouse, SendGrid
```

### Step 2: Extract entity states

Look at enum fields and status columns:

```python
class Invitation(Base):
    status = Column(Enum('pending', 'accepted', 'declined', 'expired'))
```

Becomes:
```
entity Invitation {
    status: pending | accepted | declined | expired
}
```

Look for enum definitions, status or state columns, constants like `STATUS_PENDING = 'pending'`, and state machine libraries (e.g. `transitions`, `django-fsm`).

### Step 3: Extract transitions

Find where status changes happen:

```python
def accept_invitation(invitation_id: int, slot_id: int):
    invitation = get_invitation(invitation_id)

    if invitation.status != 'pending':
        raise InvalidStateError()
    if invitation.expires_at < datetime.utcnow():
        raise ExpiredError()

    slot = get_slot(slot_id)
    if slot not in invitation.slots:
        raise InvalidSlotError()

    invitation.status = 'accepted'
    slot.status = 'booked'

    # Release other slots
    for other_slot in invitation.slots:
        if other_slot.id != slot_id:
            other_slot.status = 'available'

    # Create the interview
    interview = Interview(
        candidate_id=invitation.candidate_id,
        slot_id=slot_id,
        status='scheduled'
    )

    notify_interviewers(interview)
    send_confirmation_email(invitation.candidate, interview)
```

Extract:
```
rule CandidateAcceptsInvitation {
    when: CandidateAccepts(invitation, slot)

    requires: invitation.status = pending
    requires: invitation.expires_at > now
    requires: slot in invitation.slots

    ensures: invitation.status = accepted
    ensures: slot.status = booked
    ensures:
        for s in invitation.slots:
            if s != slot: s.status = available
    ensures: Interview.created(
        candidacy: invitation.candidacy,
        slot: slot,
        status: scheduled
    )
    ensures: Notification.created(to: slot.interviewers, ...)
    ensures: Email.created(to: invitation.candidate.email, ...)
}
```

**Key extraction patterns:**

| Code pattern | Spec pattern |
|--------------|--------------|
| `if x.status != 'pending': raise` | `requires: x.status = pending` |
| `if x.expires_at < now: raise` | `requires: x.expires_at > now` |
| `if item not in collection: raise` | `requires: item in collection` |
| `x.status = 'accepted'` | `ensures: x.status = accepted` |
| `Model.create(...)` | `ensures: Model.created(...)` |
| `send_email(...)` | `ensures: Email.created(...)` |
| `notify(...)` | `ensures: Notification.created(...)` |

Assertions, checks and validations found in code (e.g. `assert balance >= 0`, class-level validators) may map to expression-bearing invariants rather than rule preconditions. Consider whether they describe a system-wide property or a rule-specific guard.

### Step 4: Find temporal triggers

Look for scheduled jobs and time-based logic:

```python
# In celery tasks or cron jobs
@app.task
def expire_invitations():
    expired = Invitation.query.filter(
        Invitation.status == 'pending',
        Invitation.expires_at < datetime.utcnow()
    ).all()

    for invitation in expired:
        invitation.status = 'expired'
        for slot in invitation.slots:
            slot.status = 'available'
        notify_candidate_expired(invitation)

@app.task
def send_reminders():
    upcoming = Interview.query.filter(
        Interview.status == 'scheduled',
        Interview.slot.time.between(
            datetime.utcnow() + timedelta(hours=1),
            datetime.utcnow() + timedelta(hours=2)
        )
    ).all()

    for interview in upcoming:
        send_reminder_notification(interview)
```

Extract:
```
rule InvitationExpires {
    when: invitation: Invitation.expires_at <= now
    requires: invitation.status = pending

    ensures: invitation.status = expired
    ensures:
        for s in invitation.slots:
            s.status = available
    ensures: CandidateInformed(candidate: invitation.candidate, about: invitation_expired)
}

rule InterviewReminder {
    when: interview: Interview.slot.time - 1.hour <= now
    requires: interview.status = scheduled

    ensures: Notification.created(to: interview.interviewers, template: reminder)
}
```

### Step 5: Identify external boundaries

Look for third-party API calls, webhook handlers, import/export functions, and data that is read but never written (or vice versa).

These often indicate external entities:

```python
# Candidate data comes from Greenhouse, we don't create it
def import_from_greenhouse(webhook_data):
    candidate = Candidate.query.filter_by(
        greenhouse_id=webhook_data['id']
    ).first()

    if not candidate:
        candidate = Candidate(greenhouse_id=webhook_data['id'])

    candidate.name = webhook_data['name']
    candidate.email = webhook_data['email']
```

Suggests:
```
external entity Candidate {
    name: String
    email: String
}
```

When repeated interface patterns appear across service boundaries (e.g. the same serialisation contract expected by multiple consumers), these suggest `contract` declarations for reuse rather than duplicated inline obligation blocks.

### Step 6: Abstract away implementation

Now make a pass through your extracted spec and remove implementation details.

**Before (too concrete):**
```
entity Invitation {
    candidate_id: Integer
    token: String(32)
    created_at: DateTime
    expires_at: DateTime
    status: pending | accepted | declined | expired
}
```

**After (domain-level):**
```
entity Invitation {
    candidacy: Candidacy
    created_at: Timestamp
    expires_at: Timestamp
    status: pending | accepted | declined | expired

    is_expired: expires_at <= now
}
```

Changes:
- `candidate_id: Integer` became `candidacy: Candidacy` (relationship, not FK)
- `token: String(32)` removed (implementation)
- `DateTime` became `Timestamp` (domain type)
- Added derived `is_expired` for clarity

Config values that derive from other config values (e.g. `extended_timeout = base_timeout * 2`) should use qualified references or expression-form defaults in the config block rather than independent literal values.

### Step 7: Validate with stakeholders

The extracted spec is a hypothesis. Validate it:

1. **Show the spec to the original developers.** "Is this what the system does?"
2. **Show to stakeholders.** "Is this what the system should do?"
3. **Look for gaps.** Code often has bugs or missing features; the spec might reveal them.

Common findings:
- "Oh, that retry logic was a hack, we should remove it"
- "Actually we wanted X but never built it"
- "These two code paths should be the same but aren't"

## Recognising library spec candidates

During distillation, stay alert for code that implements **generic integration patterns** rather than application-specific logic. These belong in library specs, not your main specification.

The same principle applies in elicitation. When a stakeholder describes "we use Google for login" or "payments go through Stripe", pause and consider whether this is a library spec.

### Signals in the code

**Third-party integration modules:**
```python
# Finding code like this suggests a library spec
class StripeWebhookHandler:
    def handle_invoice_paid(self, event):
        ...
    def handle_subscription_cancelled(self, event):
        ...

class GoogleOAuthProvider:
    def exchange_code(self, code):
        ...
    def refresh_token(self, refresh_token):
        ...
```

**Generic patterns with specific providers:**
- OAuth flows (Google, Microsoft, GitHub)
- Payment processing (Stripe, PayPal)
- Email delivery (SendGrid, Postmark, SES)
- Calendar sync (Google Calendar, Outlook)
- ATS integrations (Greenhouse, Lever)
- File storage (S3, GCS)

**Configuration-driven integrations:**
```python
# Heavy configuration suggests the integration itself is separable
OAUTH_CONFIG = {
    'google': {'client_id': ..., 'scopes': ...},
    'microsoft': {'client_id': ..., 'scopes': ...},
}
```

### Questions to ask

1. **"Is this integration logic, or application logic?"**
   Integration: how to talk to Stripe.
   Application: what to do when payment succeeds.

2. **"Would another application integrate the same way?"**
   If yes, library spec candidate. If no, probably application-specific.

3. **"Does the code separate integration from application concerns?"**
   If cleanly separated, easy to extract to library spec. If tangled, might need refactoring first (but the spec should still separate them).

### How to handle

**Option 1: Reference an existing library spec**

If a standard library spec exists for this integration:
```
use "github.com/allium-specs/stripe-billing/abc123" as stripe

-- Application responds to Stripe events
rule ActivateSubscription {
    when: stripe/PaymentSucceeded(invoice)
    ...
}
```

**Option 2: Create a separate library spec**

If no standard spec exists but the integration is generic:
```
-- greenhouse-ats.allium (library spec)
-- Specifies: Greenhouse webhook events, candidate sync, etc.

-- interview-scheduling.allium (application spec)
use "./greenhouse-ats.allium" as greenhouse

rule ImportCandidate {
    when: greenhouse/CandidateCreated(data)
    ensures: Candidacy.created(...)
}
```

**Option 3: Abstract and move on**

If the integration is minor, just abstract it:
```
-- Don't specify Slack details, just:
ensures: Notification.created(
    to: interviewers,
    channel: slack
)
```

### Red flags: integration logic in your spec

If you find yourself writing spec like this, stop and reconsider:

```
-- TOO DETAILED - this is Stripe's domain, not yours
rule ProcessStripeWebhook {
    when: WebhookReceived(payload, signature)

    requires: verify_stripe_signature(payload, signature)

    let event = parse_stripe_event(payload)

    if event.type = "invoice.paid":
        ...
}
```

Instead:
```
-- Application responds to payment events (integration handled elsewhere)
rule PaymentReceived {
    when: stripe/InvoicePaid(invoice)
    ...
}
```

### Common library spec extractions

| Code pattern found | Library spec candidate |
|-------------------|----------------------|
| OAuth token exchange, refresh, session management | `oauth2.allium` |
| Stripe webhook handling, subscription lifecycle | `stripe-billing.allium` |
| Email sending with templates, bounce handling | `email-delivery.allium` |
| Calendar event sync, availability checking | `calendar-integration.allium` |
| ATS candidate import, status sync | `greenhouse-ats.allium`, `lever-ats.allium` |
| File upload, virus scanning, thumbnail generation | `file-storage.allium` |

See patterns.md Pattern 8 for detailed examples of integrating library specs.

## Common distillation challenges

### Challenge: Duplicate terminology

When you find two terms for the same concept (across specs, within a spec, or between spec and code) treat it as a blocking problem.

```
-- BAD: Acknowledges duplication without resolving it
-- Order vs Purchase
-- checkout.allium uses "Purchase" - these are equivalent concepts.
```

This is not a resolution. When different parts of a codebase are built against different specs, both terms end up in the implementation: duplicate models, redundant join tables, foreign keys pointing both ways.

**What to do:**
- Choose one term. Cross-reference related specs before deciding.
- Update all references. Do not leave the old term in comments or "see also" notes.
- Note the rename in a changelog, not in the spec itself.

**Warning signs in code:**
- Two models representing the same concept (`Order` and `Purchase`)
- Join tables for both (`order_items`, `purchase_items`)
- Comments like "equivalent to X" or "same as Y"

The spec you extract must pick one term. Flag the other as technical debt to remove.

### Challenge: Implicit state machines

Code often has implicit states that are not modelled:

```python
# No explicit status field, but there's a state machine hiding here
class FeedbackRequest:
    interview_id = Column(Integer)
    interviewer_id = Column(Integer)
    requested_at = Column(DateTime)
    reminded_at = Column(DateTime, nullable=True)
    feedback_id = Column(Integer, nullable=True)  # FK to Feedback if submitted
```

The implicit states are:
- `pending`: requested_at set, feedback_id null, reminded_at null
- `reminded`: reminded_at set, feedback_id null
- `submitted`: feedback_id set

Extract to explicit:
```
entity FeedbackRequest {
    interview: Interview
    interviewer: Interviewer
    requested_at: Timestamp
    reminded_at: Timestamp?
    status: pending | reminded | submitted
}
```

### Challenge: Scattered logic

The same conceptual rule might be spread across multiple places:

```python
# In API handler
def accept_invitation(request):
    if invitation.status != 'pending':
        return error(400, "Already responded")
    ...

# In model
class Invitation:
    def can_accept(self):
        return self.expires_at > datetime.utcnow()

# In service
def process_acceptance(invitation, slot):
    if slot not in invitation.slots:
        raise InvalidSlot()
    ...
```

Consolidate into one rule:
```
rule CandidateAccepts {
    when: CandidateAccepts(invitation, slot)

    requires: invitation.status = pending
    requires: invitation.expires_at > now
    requires: slot in invitation.slots
    ...
}
```

### Challenge: Dead code and historical accidents

Codebases accumulate features that were built but never used, workarounds for bugs that are now fixed, and code paths that are never executed.

Do not include these in the spec. If you are unsure:
1. Check if the code is actually reachable
2. Ask developers if it is intentional
3. Check git history for context

### Challenge: Missing error handling

Code might silently fail or have incomplete error handling:

```python
def send_notification(user, message):
    try:
        slack.send(user.slack_id, message)
    except SlackError:
        pass  # Silently ignore failures
```

The spec should capture the intended behaviour, not the bug:
```
ensures: Notification.created(to: user, channel: slack)
```

Whether the current implementation properly handles failures is separate from what the system should do.

### Challenge: Over-engineered abstractions

Enterprise codebases often have abstraction layers that obscure intent:

```java
public interface NotificationStrategy {
    void notify(NotificationContext context);
}

public class SlackNotificationStrategy implements NotificationStrategy {
    @Override
    public void notify(NotificationContext context) {
        // Actual Slack call buried 5 levels deep
    }
}
```

Cut through to the actual behaviour. The spec does not need strategy patterns, dependency injection or abstract factories. Just: `ensures: Notification.created(channel: slack, ...)`

## Checklist: Have you abstracted enough?

Before finalising a distilled spec:

- [ ] No database column types (Integer, VARCHAR, etc.)
- [ ] No ORM or query syntax
- [ ] No HTTP status codes or API paths
- [ ] No framework-specific concepts (middleware, decorators, etc.)
- [ ] No programming language types (int, str, List, etc.)
- [ ] No variable names from the code (use domain terms)
- [ ] No infrastructure (Redis, Kafka, S3, etc.)
- [ ] Foreign keys replaced with relationships
- [ ] Tokens/secrets removed (implementation of identity)
- [ ] Timestamps use domain Duration, not timedelta/seconds

If any remain, ask: "Would a stakeholder include this in a requirements doc?"

## Checklist: Terminology consistency

- [ ] Each concept has exactly one name throughout the spec
- [ ] No "also known as" or "equivalent to" comments
- [ ] Cross-referenced related specs for conflicting terms
- [ ] Duplicate models in code flagged as technical debt to remove

## After distillation

The extracted spec is a starting point. For targeted changes as requirements evolve, use the `tend` agent. For checking ongoing alignment between the spec and implementation, use the `weed` agent.

## References

- [Language reference](../../references/language-reference.md), full Allium syntax
- [Worked examples](./references/worked-examples.md), complete code-to-spec examples in Python, TypeScript and Java
