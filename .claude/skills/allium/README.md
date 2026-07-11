# Allium

*Velocity through clarity*

---

Give your AI agents something more useful than a prompt. [juxt.github.io/allium](https://juxt.github.io/allium/)

## Get started

**Claude Code** (via the JUXT plugin marketplace):

```
/plugin marketplace add juxt/claude-plugins
/plugin install allium
```

**Cursor, Windsurf, Copilot, Aider, Continue and 40+ other tools:**

```
npx skills add juxt/allium
```

Once installed, type `/allium` to get started. Allium examines your project and offers to distill from existing code or build a new spec through conversation. You can also jump straight to a specific mode:

- `/allium:elicit` — build a spec through structured conversation with stakeholders
- `/allium:distill` — extract a spec from existing code
- `/allium:propagate` — generate tests from a specification

Tend and propagate are Allium's built-in agent and skill. Tend updates your specs, propagate generates tests from them. Both work with whatever LLM tooling you have installed.

Jump to what [Allium looks like in practice](#what-this-looks-like-in-practice).

## Working with agents

A [rule](.claude/rules/allium.md) loads automatically whenever Claude Code works with `.allium` files. It provides syntax guidance, naming conventions and common pitfalls without needing to invoke the skill. This keeps routine spec reads and edits fast.

Two specialised agents handle `.allium` files in a delegated context. They load the language reference into their own conversation, keeping Allium syntax out of your main session and freeing you to work on implementation in parallel.

**[tend](.claude/agents/tend.md)** grows and shapes specifications. It translates new requirements into well-formed specs, challenges vague requests and won't let ambiguity through. It works on `.allium` files only.

```
"Tend: we need a cancellation policy for subscriptions
 with a cooling-off period and prorated refunds"

"Tend: add a circuit breaker entity to the infrastructure spec"

"Tend: restructure the authentication spec, the rules have grown unwieldy"
```

**[weed](.claude/agents/weed.md)** finds where specifications and implementation have diverged. It reports mismatches and can update either side to match.

```
"Weed the auth spec against src/auth/"

"Weed: update the spec to match what the payment code actually does"

"Weed: the order spec says cancelled orders can't be refunded
 but the code allows it. Fix the code."
```

Use `/allium` for interactive spec work, `elicit` to build specs through conversation, `distill` to extract specs from code, `tend` to grow specs as requirements evolve, `weed` to catch drift between spec and code.

## The problem with conversational context

- Within a session, meaning drifts: by prompt ten or twenty, the model is pattern-matching on its own outputs rather than the original intent.
- Across sessions, knowledge evaporates: assumptions and constraints disappear when the chat ends.

Allium gives behavioural intent a durable form that doesn't drift with the conversation and persists across sessions.

## Why not just point the LLM at the code?

Modern LLMs navigate codebases effectively, and many engineers find this sufficient. The limitation appears when you need to distinguish what the code *does* from what it *should do*. Code captures implementation, including bugs and expedient decisions. The model treats all of it as intended behaviour.

Precise prompting helps, but precise prompting means specifying intent: which behaviours are deliberate, which constraints must be preserved. You end up writing descriptions of intent distributed across your prompts. Allium captures this in a form that persists. The next engineer, or the next model, or you next week, can understand not just what the system does but what it was meant to do.

## Why not capture requirements in markdown?

Markdown provides no framework for surfacing ambiguities and contradictions. You can write "users must be authenticated" in one section and "guest checkout is supported" in another without the format highlighting the tension. Capable models may resolve such ambiguities silently in ways you didn't intend; weaker models may not recognise that alternatives existed.

Allium's structure makes contradictions visible. When two rules have incompatible preconditions, the formal syntax exposes the conflict. The model doesn't need to be clever enough to spot the issue in prose; the structure does that work. Markdown can capture robust behaviour with sufficient diligence, but that diligence falls entirely on the author. Allium's constraints guide you toward completeness and consistency.

## Iterating on specifications

The specification and the code evolve together. Writing and refining a behavioural model alongside implementation sharpens your understanding of both the problem and your solution. Questions surface that you wouldn't have thought to ask; constraints emerge that only become visible when you try to formalise them.

Manual coding embedded this discovery in the act of implementation. LLMs generate code from descriptions, shifting where design thinking occurs. Allium captures it explicitly: the specification becomes the site of that thinking, the code its expression.

Two processes feed this growth: **elicitation** works forward from intent through structured conversations with stakeholders, while **distillation** works backward from implementation to capture what the system actually does, including behaviours that were never explicitly decided. Distillation reveals what you built; elicitation clarifies what you meant. When these diverge, you've found something worth investigating.

See the [elicitation guide](skills/elicit/SKILL.md) and the [distillation guide](skills/distill/SKILL.md) for detailed approaches.

## On single sources of truth

A common objection is that maintaining behavioural models alongside code violates the single source of truth principle. But code captures both intentional and accidental behaviour, with no mechanism to distinguish them. Is that authentication quirk a feature or a bug? The code can't tell you. You need something outside the code to even articulate "this behaviour is wrong". Engineers already accept this in other contexts: type systems express intent that code must satisfy, tests assert expected behaviour against actual behaviour. These aren't duplication.

Allium applies the same pattern. Code excels at expressing *how*; behavioural models excel at expressing *what* and *under which conditions*. When these disagree, that disagreement is information. Perhaps the implementation drifted from intent, or perhaps the model was naive. Either might need to change. The gap between them surfaces questions you need to answer. Redundancy, in this context, isn't overhead. It's resilience.

## What Allium captures

Allium provides a minimal syntax for describing events with their preconditions and the outcomes that result. The language deliberately excludes implementation details such as database schemas and API designs, focusing purely on observable behaviour.

```allium
rule RequestPasswordReset {
    when: UserRequestsPasswordReset(email)

    let user = User{email}

    requires: exists user
    requires: user.status in {active, locked}

    ensures:
        for t in user.pending_reset_tokens:
            t.status = expired
    ensures:
        let token = PasswordResetToken.created(
            user: user,
            created_at: now,
            expires_at: now + config.reset_token_expiry,
            status: pending
        )
        Email.created(
            to: user.email,
            template: password_reset,
            data: { token: token }
        )
}
```

This rule captures observable behaviour: when a password reset is requested, if the email matches an active or locked account, existing tokens are invalidated, a new token is created and an email is sent. It says nothing about which database stores the token or which service sends the email, because those decisions belong to implementation.

The same syntax works whether you're capturing infrastructure contracts or operational policy. A circuit breaker specification describes behaviour that typically lives in library defaults, Grafana alerts and architecture docs, never in any formal specification:

```allium
entity CircuitBreaker {
    service: ExternalService
    status: closed | open | half_open
    opened_at: Timestamp?
    failures: Failure with circuit_breaker = this
    recent_failures: failures where occurred_at > now - config.failure_window
    failure_rate: recent_failures.count / config.window_sample_size
    is_tripped: failure_rate >= config.failure_threshold
}

config {
    failure_threshold: Decimal = 0.5
    failure_window: Duration = 30.seconds
    window_sample_size: Integer = 20
    recovery_timeout: Duration = 10.seconds
}

rule CircuitOpens {
    when: circuit_breaker: CircuitBreaker.is_tripped
    requires: circuit_breaker.status = closed

    ensures:
        circuit_breaker.status = open
        circuit_breaker.opened_at = now
}

rule CircuitProbes {
    when: circuit_breaker: CircuitBreaker.opened_at + config.recovery_timeout <= now
    requires: circuit_breaker.status = open

    ensures: circuit_breaker.status = half_open
}
```

At the other end, an incident escalation rule captures operational policy that otherwise lives in runbooks, PagerDuty config and tribal knowledge, where drift between intent and implementation causes real damage:

```allium
config {
    exec_notify_threshold: Integer = 2
}

deferred EscalationPolicy.at_level

rule IncidentEscalates {
    when: incident: Incident.declared_at + incident.sla_target <= now
    requires: incident.status in {open, investigating}

    ensures:
        incident.escalation_level = incident.escalation_level + 1
        OnCallPaged(
            team: EscalationPolicy.at_level(incident.escalation_level),
            priority: immediate
        )
        if incident.escalation_level >= config.exec_notify_threshold:
            ExecBriefingSent(incident: incident)
}
```

The [language reference](references/language-reference.md) covers entities, rules, triggers, relationships, projections, derived values, surfaces and actor declarations. The [v1 to v2 migration guide](references/migration-v1-to-v2.md) and [v2 to v3 migration guide](references/migration-v2-to-v3.md) cover what changed between versions and how to upgrade existing specs.

### A language without a runtime

Allium has no compiler and no runtime. It is purely descriptive, defined entirely by its documentation.

In an era where LLMs function as pseudocode compilers, executing informal descriptions into working code, a well-structured behavioural language becomes the mechanism for ensuring that what gets compiled is what you actually meant. The behavioural model is the primary artefact; the code that implements it is secondary.

## What this looks like in practice

### Allium surfaces implications you missed

A customer's account was suspended for a terms of service violation. They've appealed and the team wants to let them back in.

---

> ***> A customer says they can't reset their password. Their account was suspended last month but they've appealed and we want to let them back in. Can you enable password reset for suspended users?***
>
> ✻ Your Allium requires `user.status in {active, locked}` for password resets. This user's status is `suspended`, so the reset is blocked by design. Reactivating through password reset would bypass your `AccountReinstated` rule, which requires admin approval and logs an audit trail.
>
> ✻ Should we reinstate the account through the existing admin flow first, then let them reset their password? Or create a new `AccountReinstatedViaAppeal` rule that combines both steps with the audit trail intact?
>
> ***> Reinstate through the admin flow first. The audit trail matters.***

---

Without Allium, the LLM would have relaxed the status check, accidentally creating a backdoor around an admin control. The behavioural model caught that password reset and account reinstatement are separate concerns with different approval requirements.

### Knowledge persists across sessions

A developer starts a new session days later, working on a different feature.

---

> ***> Add a free trial. New subscribers get their first month at no charge.***
>
> ✻ Your Allium has a constraint here. `SubscriptionCreated` ensures a payment is processed and `InvoiceGenerated` requires `invoice.amount > 0`. A free first month would need to bypass both.
>
> ✻ Should the trial skip invoice generation entirely, or create a zero-value invoice for the audit trail? And should the `PaymentMethod` still be captured upfront, or only when the trial converts?
>
> ***> Create a zero-value invoice and capture the payment method upfront.***

---

The developer never mentioned invoicing or payment method capture. The Allium did, from a session that ended days ago. Without it, the LLM would have implemented the trial with no awareness of these constraints, and the gaps would have surfaced in production.

### Allium grounds a design conversation

A developer asks for a straightforward feature.

---

> ***> Add an endpoint so admins can view any user's payment history.***
>
> ✻ Your Allium doesn't give admins blanket access. `AdminAction` requires the admin's scope to include the target resource. The current scopes are `billing`, `users` and `support`. Should this endpoint require `billing` scope?
>
> ***> Yes, billing scope. But our support team needs to see payment history too, for resolving disputes. What are our options?***
>
> ✻ You could add `billing` to the support role, but that also grants access to refunds and plan changes, which your Allium controls through the same scope.
>
> ✻ Alternatively, split `billing` into `billing.read` and `billing.write`. Support gets read-only access, finance gets both. Your code in `admin/permissions.py` already distinguishes read and write operations but maps them both to the same scope check, so the implementation is halfway there.
>
> ***> Go with the read/write split. Update the Allium first, then refactor the scope checks to match.***

---

Without Allium, the LLM would likely have implemented a simple `is_admin` check. Instead, the conversation surfaced a scoping model, explored its implications for a second team, and arrived at a design that aligns the behavioural model with a distinction the code already made but never formalised.

### Distillation catches drift

A developer asks the LLM to check whether the code still matches the Allium.

---

> ***> Can you check whether our authentication code matches the Allium?***
>
> ✻ I found a divergence. Your Allium says `LoginFailure` checks `user.failed_login_attempts >= config.max_login_attempts`, configured as 5. But `auth/login.py` line 47 hardcodes the limit at 3 and doesn't reset the counter after a successful login. The Allium says `LoginSuccess` ensures `user.failed_login_attempts = 0`.
>
> ✻ Either the code drifted or the Allium is aspirational. Which should we trust?
>
> ***> The Allium is right. Fix the code to match.***

---

Code and intent diverge silently over time. Allium gives the LLM something to check against, turning "does this look right?" into a concrete comparison with a definitive answer.

## Verification

When the [Allium CLI](https://github.com/juxt/allium-tools) is installed, `.allium` files are validated automatically after every write or edit in Claude Code. Install it via Homebrew (`brew tap juxt/allium && brew install allium`) or Cargo (`cargo install allium-cli`). Diagnostics appear inline and the model fixes issues in the same turn.

Without the CLI, the skill falls back to validating against the language reference. The CLI catches more, particularly parser-level errors and cross-entity reference checks, so installing it is recommended if you're working with Allium regularly.

## Language governance

Every change to Allium is debated by a [nine-member review panel](https://github.com/juxt/allium/blob/proposals/TEAM.md) before adoption. Each panellist represents a distinct design priority: simplicity, machine reasoning, composability, readability, formal rigour, domain modelling, developer experience, creative ambition and backward compatibility. The panel exists to surface tensions that any single perspective would miss.

The panel operates in two modes. [Reviews](https://github.com/juxt/allium/blob/proposals/REVIEW.md) evaluate fixes to rough edges in the existing language, where the default is to fix the problem if a good fix exists. [Proposals](https://github.com/juxt/allium/blob/proposals/PROPOSE.md) evaluate new features and ambitious changes, where the default is to leave the language alone unless the case for change is strong. Both follow the same debate protocol: present, respond, rebut, synthesise, verdict.

## Feedback

We'd love to hear how you get on with Allium. Success stories, rough edges, missing features, things that surprised you. Drop us a line at [info@juxt.pro](mailto:info@juxt.pro) or [raise an issue](https://github.com/juxt/allium/issues) if you have a specific request.

## About the name

Allium is the botanical family containing onions and shallots. The name continues a tradition in behaviour specification tooling: Cucumber and Gherkin established botanical naming as a convention in behaviour-driven development, followed by tools like Lettuce and Spinach.

The phonetic echo of "LLM" is intentional, reflecting where we expect these models to be most useful.

The idiom "know your onions" means to understand a subject thoroughly. Engineers have always held two models: what the system should do and what it currently does. Code formalised implementation; intent remained scattered across documents, emails and Slack messages. LLMs generate implementations from descriptions, so Allium consolidates that scattered understanding into an explicit form models can reference reliably.

Like its namesake, working with Allium may produce tears during the peeling, but never at the table.

## Star History

<a href="https://www.star-history.com/?repos=juxt%2Fallium&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/image?repos=juxt/allium&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/image?repos=juxt/allium&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/image?repos=juxt/allium&type=date&legend=top-left" />
 </picture>
</a>

---

## Copyright & License

The MIT License (MIT)

Copyright © 2026 JUXT Ltd.

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
