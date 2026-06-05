# Complete patterns

This library contains reusable patterns for common SaaS scenarios. Each pattern demonstrates specific Allium language features and can be adapted to your domain.

Patterns elide common cross-cutting entities (`Email`, `Notification`, `AuditLog`, etc.) for brevity. In a real specification, declare these as external entities or define them in a shared module.

| Pattern | Key Features Demonstrated |
|---------|---------------------------|
| Password Auth with Reset | Temporal triggers, token lifecycle, defaults, surfaces |
| Role-Based Access Control | Derived permissions, relationships, `requires` checks, surfaces |
| Invitation to Resource | Join entities, permission levels, tokenised actions, surfaces |
| Soft Delete & Restore | State machines, projections filtering deleted items |
| Notification Preferences | Sum types for notification variants, user preferences, digest batching, surfaces |
| Usage Limits & Quotas | Limit checks in `requires`, metered resources, plan tiers, surfaces |
| Comments with Mentions | Nested entities, parsing triggers, cross-entity notifications, surfaces |
| Integrating Library Specs | External spec references, configuration, config parameter references, responding to external triggers |
| Framework Integration Contract | Contract declarations, expression-bearing invariants, contract references, programmatic surfaces |

---

## Pattern 1: Password Authentication with Reset

**Demonstrates:** Temporal triggers, token lifecycle, defaults, surfaces, multiple related rules

This pattern handles user registration, login and password reset: the foundation of most SaaS applications.

```
-- allium: 3
-- password-auth.allium

config {
    min_password_length: Integer = 12
    max_login_attempts: Integer = 5
    lockout_duration: Duration = 15.minutes
    reset_token_expiry: Duration = 1.hour
    session_duration: Duration = 24.hours
}

------------------------------------------------------------
-- Entities
------------------------------------------------------------

entity User {
    email: String
    password_hash: String          -- stored, never exposed
    status: active | locked | deactivated
    failed_login_attempts: Integer
    locked_until: Timestamp?

    -- Relationships
    sessions: Session with user = this
    reset_tokens: PasswordResetToken with user = this

    -- Projections
    active_sessions: sessions where status = active
    pending_reset_tokens: reset_tokens where status = pending

    -- Derived
    is_locked: status = locked and locked_until > now
}

entity Session {
    user: User
    created_at: Timestamp
    expires_at: Timestamp
    status: active | expired | revoked

    -- Derived
    is_valid: status = active and expires_at > now
}

entity PasswordResetToken {
    user: User
    created_at: Timestamp
    expires_at: Timestamp
    status: pending | used | expired

    -- Derived
    is_valid: status = pending and expires_at > now
}

------------------------------------------------------------
-- Registration
------------------------------------------------------------

rule Register {
    when: UserRegisters(email, password)

    requires: not exists User{email: email}
    requires: length(password) >= config.min_password_length

    ensures: User.created(
        email: email,
        password_hash: hash(password),    -- black box
        status: active,
        failed_login_attempts: 0
    )
    ensures: Email.created(
        to: email,
        template: welcome
    )
}

------------------------------------------------------------
-- Login
------------------------------------------------------------

rule LoginSuccess {
    when: UserLogsIn(email, password)

    let user = User{email}

    requires: exists user
    requires: not user.is_locked
    requires: verify(password, user.password_hash)    -- black box

    ensures: user.failed_login_attempts = 0
    ensures: Session.created(
        user: user,
        created_at: now,
        expires_at: now + config.session_duration,
        status: active
    )
}

rule LoginFailure {
    when: UserLogsIn(email, password)

    let user = User{email}

    requires: exists user
    requires: not user.is_locked
    requires: not verify(password, user.password_hash)

    ensures: user.failed_login_attempts = user.failed_login_attempts + 1
    ensures:
        if user.failed_login_attempts >= config.max_login_attempts:
            user.status = locked
            user.locked_until = now + config.lockout_duration
            Email.created(to: user.email, template: account_locked)
}

rule LoginAttemptWhileLocked {
    when: UserLogsIn(email, password)

    let user = User{email}

    requires: exists user
    requires: user.is_locked

    ensures: UserInformed(
        user: user,
        about: account_locked,
        data: { unlocks_at: user.locked_until }
    )
}

rule LockoutExpires {
    when: user: User.locked_until <= now

    requires: user.status = locked

    ensures: user.status = active
    ensures: user.failed_login_attempts = 0
    ensures: user.locked_until = null
}

------------------------------------------------------------
-- Logout
------------------------------------------------------------

rule Logout {
    when: UserLogsOut(session)

    requires: session.status = active

    ensures: session.status = revoked
}

rule SessionExpires {
    when: session: Session.expires_at <= now

    requires: session.status = active

    ensures: session.status = expired
}

------------------------------------------------------------
-- Password Reset
------------------------------------------------------------

rule RequestPasswordReset {
    when: UserRequestsPasswordReset(email)

    let user = User{email}

    requires: exists user
    requires: user.status in {active, locked}

    -- Invalidate any existing tokens
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
            to: email,
            template: password_reset,
            data: { token: token }
        )
}

rule CompletePasswordReset {
    when: UserResetsPassword(token, new_password)

    requires: token.is_valid
    requires: length(new_password) >= config.min_password_length

    let user = token.user

    ensures: token.status = used
    ensures: user.password_hash = hash(new_password)
    ensures: user.status = active
    ensures: user.failed_login_attempts = 0
    ensures: user.locked_until = null

    -- Invalidate all existing sessions
    ensures:
        for s in user.active_sessions:
            s.status = revoked

    ensures: Email.created(
        to: user.email,
        template: password_changed
    )
}

rule ResetTokenExpires {
    when: token: PasswordResetToken.expires_at <= now

    requires: token.status = pending

    ensures: token.status = expired
}

------------------------------------------------------------
-- Actors
------------------------------------------------------------

actor AuthenticatedUser {
    identified_by: User where active_sessions.count > 0
}

------------------------------------------------------------
-- Surfaces
------------------------------------------------------------

surface Authentication {
    facing visitor: User

    provides:
        UserLogsIn(email, password)
        UserRegisters(email, password)
        UserRequestsPasswordReset(email)

    @guarantee NoSessionRequired
        -- Accessible without an existing session.

    @guidance
        -- Show lockout status and unlock time when user.is_locked.
        -- Validate password length client-side before submission.
}

surface PasswordReset {
    facing visitor: User

    context token: PasswordResetToken

    exposes:
        token.is_valid
        token.expires_at

    provides:
        UserResetsPassword(token, new_password)
            when token.is_valid

    @guarantee NoSessionRequired
        -- Accessible without an existing session.
}

surface AccountManagement {
    facing user: AuthenticatedUser

    exposes:
        user.email
        user.active_sessions
        user.active_sessions.count

    provides:
        for session in user.active_sessions:
            UserLogsOut(session)
        UserRequestsPasswordReset(user.email)
}
```

**Key language features shown:**
- `config` block for configurable parameters (`config.min_password_length`, etc.)
- Derived values (`is_locked`, `is_valid`)
- Multiple rules for same trigger with different `requires` (login success vs failure)
- Temporal triggers with guards (`when: token: PasswordResetToken.expires_at <= now` with `requires: status = pending`)
- Projections for filtered collections (`pending_reset_tokens`)
- Bulk updates with `for` iteration
- Explicit `let` binding for created entities
- Black box functions (`hash()`, `verify()`)
- Surfaces with `facing` declaration and `for` iteration in `provides`

---

## Pattern 2: Role-Based Access Control (RBAC)

**Demonstrates:** Derived permissions, relationships, using permissions in `requires` clauses, surfaces

This pattern implements hierarchical roles where higher roles inherit permissions from lower ones.

```
-- allium: 3
-- rbac.allium

------------------------------------------------------------
-- Entities
------------------------------------------------------------

entity Role {
    name: String                    -- e.g., "viewer", "editor", "admin"
    permissions: Set<String>        -- e.g., { "documents.read", "documents.write" }
    inherits_from: Role?            -- optional parent role

    -- Derived: all permissions including inherited
    effective_permissions:
        permissions + (inherits_from?.effective_permissions ?? {})
}

entity User {
    email: String
    name: String
}

entity Workspace {
    name: String
    owner: User

    -- Relationships
    memberships: WorkspaceMembership with workspace = this
    documents: Document with workspace = this

    -- Projections
    members: memberships -> user
    admins: memberships where role.name = "admin" -> user
}

entity Document {
    workspace: Workspace
    created_by: User
    title: String
    content: String
}

entity DocumentView {
    user: User
    document: Document
    at: Timestamp
}

-- Join entity connecting User, Workspace, and Role
entity WorkspaceMembership {
    user: User
    workspace: Workspace
    role: Role
    joined_at: Timestamp

    -- Derived: check specific permissions
    can_read: "documents.read" in role.effective_permissions
    can_write: "documents.write" in role.effective_permissions
    can_admin: "workspace.admin" in role.effective_permissions
}

------------------------------------------------------------
-- Defaults
------------------------------------------------------------

default Role viewer = {
    name: "viewer",
    permissions: { "documents.read" }
}

default Role editor = {
    name: "editor",
    permissions: { "documents.write" },
    inherits_from: viewer
}

default Role admin = {
    name: "admin",
    permissions: { "workspace.admin", "members.manage" },
    inherits_from: editor
}

------------------------------------------------------------
-- Rules
------------------------------------------------------------

rule CreateWorkspace {
    when: UserCreatesWorkspace(user, name)

    ensures:
        let workspace = Workspace.created(
            name: name,
            owner: user
        )
        -- Owner automatically becomes admin
        WorkspaceMembership.created(
            user: user,
            workspace: workspace,
            role: admin,
            joined_at: now
        )
}

rule AddMember {
    when: AddMemberToWorkspace(actor, workspace, new_user, role)

    let actor_membership = WorkspaceMembership{user: actor, workspace: workspace}

    requires: actor_membership.can_admin
    requires: not exists WorkspaceMembership{user: new_user, workspace: workspace}

    ensures: WorkspaceMembership.created(
        user: new_user,
        workspace: workspace,
        role: role,
        joined_at: now
    )
    ensures: Email.created(
        to: new_user.email,
        template: added_to_workspace,
        data: { workspace: workspace, role: role }
    )
}

rule ChangeMemberRole {
    when: ChangeMemberRole(actor, workspace, target_user, new_role)

    let actor_membership = WorkspaceMembership{user: actor, workspace: workspace}
    let target_membership = WorkspaceMembership{user: target_user, workspace: workspace}

    requires: actor_membership.can_admin
    requires: exists target_membership
    requires: target_user != workspace.owner    -- can't change owner's role

    ensures: target_membership.role = new_role
}

rule RemoveMember {
    when: RemoveMemberFromWorkspace(actor, workspace, target_user)

    let actor_membership = WorkspaceMembership{user: actor, workspace: workspace}
    let target_membership = WorkspaceMembership{user: target_user, workspace: workspace}

    requires: actor_membership.can_admin
    requires: exists target_membership
    requires: target_user != workspace.owner    -- can't remove owner

    ensures: not exists target_membership
}

rule LeaveWorkspace {
    when: UserLeavesWorkspace(user, workspace)

    let membership = WorkspaceMembership{user, workspace}

    requires: exists membership
    requires: user != workspace.owner    -- owner can't leave

    ensures: not exists membership
}

------------------------------------------------------------
-- Managing permissions on roles
------------------------------------------------------------

rule GrantPermission {
    when: GrantPermission(actor, workspace, role, permission)

    let actor_membership = WorkspaceMembership{user: actor, workspace: workspace}

    requires: actor_membership.can_admin
    requires: permission not in role.effective_permissions

    ensures: role.permissions.add(permission)
}

rule RevokePermission {
    when: RevokePermission(actor, workspace, role, permission)

    let actor_membership = WorkspaceMembership{user: actor, workspace: workspace}

    requires: actor_membership.can_admin
    requires: permission in role.permissions    -- only direct, not inherited

    ensures: role.permissions.remove(permission)
}

------------------------------------------------------------
-- Using permissions in other rules
------------------------------------------------------------

rule CreateDocument {
    when: CreateDocument(user, workspace, title, content)

    let membership = WorkspaceMembership{user, workspace}

    requires: membership.can_write

    ensures: Document.created(
        workspace: workspace,
        created_by: user,
        title: title,
        content: content
    )
}

rule ViewDocument {
    when: ViewDocument(user, document)

    let membership = WorkspaceMembership{user: user, workspace: document.workspace}

    requires: membership.can_read

    ensures: DocumentView.created(user: user, document: document, at: now)
}

------------------------------------------------------------
-- Actors
------------------------------------------------------------

actor WorkspaceAdmin {
    within: Workspace
    identified_by: User where WorkspaceMembership{user: this, workspace: within}.can_admin = true
}

actor WorkspaceEditor {
    within: Workspace
    identified_by: User where WorkspaceMembership{user: this, workspace: within}.can_write = true
}

actor WorkspaceViewer {
    within: Workspace
    identified_by: User where WorkspaceMembership{user: this, workspace: within}.can_read = true
}

------------------------------------------------------------
-- Surfaces
------------------------------------------------------------

surface WorkspaceMemberManagement {
    facing admin: WorkspaceAdmin

    context workspace: Workspace

    exposes:
        workspace.name
        workspace.memberships
        workspace.admins

    provides:
        AddMemberToWorkspace(admin, workspace, new_user, role)
        ChangeMemberRole(admin, workspace, target_user, new_role)
            when target_user != workspace.owner
        RemoveMemberFromWorkspace(admin, workspace, target_user)
            when target_user != workspace.owner

    @guarantee OwnerProtection
        -- The workspace owner's role cannot be changed or removed.
}

surface WorkspaceDocuments {
    facing member: User

    context workspace: Workspace

    let membership = WorkspaceMembership{user: member, workspace: workspace}

    exposes:
        workspace.name
        workspace.documents

    provides:
        CreateDocument(member, workspace, title, content)
            when membership.can_write
        for document in workspace.documents:
            ViewDocument(member, document)
                when membership.can_read

    related:
        WorkspaceMemberManagement(workspace)
            when membership.can_admin
}
```

**Key language features shown:**
- Recursive derived values (`effective_permissions` includes inherited)
- Null-safe navigation (`inherits_from?.effective_permissions ?? {}`)
- Join entity lookup (`WorkspaceMembership{user: actor, workspace: workspace}`)
- Permission checks in `requires` clauses
- String set membership with `in` operator
- `.add()` and `.remove()` for set mutation in ensures clauses
- `not exists` as an outcome (removes the entity)
- Surfaces with role-based actors and permission-gated actions
- `related` clause for cross-surface navigation

---

## Pattern 3: Invitation to Resource

**Demonstrates:** Tokenised actions, permission levels, invitation lifecycle, guest vs member flows, surfaces

This pattern handles inviting users to collaborate on resources, whether they're existing users or not.

```
-- allium: 3
-- resource-invitation.allium

config {
    invitation_expiry: Duration = 7.days
}

------------------------------------------------------------
-- Enumerations
------------------------------------------------------------

enum Permission { view | edit | admin }

------------------------------------------------------------
-- Entities
------------------------------------------------------------

entity Resource {
    name: String
    owner: User

    -- Relationships
    shares: ResourceShare with resource = this
    invitations: ResourceInvitation with resource = this

    -- Projections
    active_shares: shares where status = active
    pending_invitations: invitations where status = pending
}

entity ResourceShare {
    resource: Resource
    user: User
    permission: Permission
    status: active | revoked
    created_at: Timestamp

    -- Derived
    can_view: permission in {view, edit, admin}
    can_edit: permission in {edit, admin}
    can_admin: permission = admin
    can_invite: permission in {edit, admin}    -- editors and admins can invite
}

entity ResourceInvitation {
    resource: Resource
    email: String
    permission: Permission
    invited_by: User
    created_at: Timestamp
    expires_at: Timestamp
    status: pending | accepted | declined | expired | revoked

    -- Derived
    is_valid: status = pending and expires_at > now
}

------------------------------------------------------------
-- Inviting
------------------------------------------------------------

rule InviteToResource {
    when: InviteToResource(inviter, resource, email, permission)

    let inviter_share = ResourceShare{resource: resource, user: inviter}
    let existing_invitation = ResourceInvitation{resource: resource, email: email}

    requires: inviter = resource.owner or inviter_share.can_invite
    requires: permission in {view, edit}    -- can't invite as admin unless owner
              or (permission = admin and inviter = resource.owner)
    requires: not exists ResourceShare{resource: resource, user: User{email: email}}
    requires: not exists existing_invitation or not existing_invitation.is_valid

    ensures: ResourceInvitation.created(
        resource: resource,
        email: email,
        permission: permission,
        invited_by: inviter,
        created_at: now,
        expires_at: now + config.invitation_expiry,
        status: pending
    )
    ensures: Email.created(
        to: email,
        template: resource_invitation,
        data: {
            resource: resource,
            inviter: inviter,
            permission: permission
        }
    )
}

------------------------------------------------------------
-- Accepting (existing user)
------------------------------------------------------------

rule AcceptInvitationExistingUser {
    when: ExistingUserAcceptsInvitation(invitation, user)

    requires: invitation.is_valid
    requires: user.email = invitation.email

    ensures: invitation.status = accepted
    ensures: ResourceShare.created(
        resource: invitation.resource,
        user: user,
        permission: invitation.permission,
        status: active,
        created_at: now
    )
    ensures: Notification.created(
        to: invitation.invited_by,
        template: invitation_accepted,
        data: { resource: invitation.resource, user: user }
    )
}

------------------------------------------------------------
-- Accepting (new user - triggers signup flow)
------------------------------------------------------------

rule AcceptInvitationNewUser {
    when: NewUserAcceptsInvitation(invitation, email, name, password)

    requires: invitation.is_valid
    requires: email = invitation.email
    requires: not exists User{email: email}

    ensures:
        let user = User.created(
            email: email,
            name: name,
            password_hash: hash(password),
            status: active
        )
        invitation.status = accepted
        ResourceShare.created(
            resource: invitation.resource,
            user: user,
            permission: invitation.permission,
            status: active,
            created_at: now
        )
        Notification.created(
            to: invitation.invited_by,
            template: invitation_accepted,
            data: { resource: invitation.resource, user: user }
        )
}

------------------------------------------------------------
-- Declining and expiring
------------------------------------------------------------

rule DeclineInvitation {
    when: DeclineInvitation(invitation)

    requires: invitation.is_valid

    ensures: invitation.status = declined
}

rule InvitationExpires {
    when: invitation: ResourceInvitation.expires_at <= now

    requires: invitation.status = pending

    ensures: invitation.status = expired
}

rule RevokeInvitation {
    when: RevokeInvitation(actor, invitation)

    let actor_share = ResourceShare{resource: invitation.resource, user: actor}

    requires: invitation.status = pending
    requires: actor = invitation.resource.owner or actor_share.can_admin

    ensures: invitation.status = revoked
}

------------------------------------------------------------
-- Managing shares
------------------------------------------------------------

rule ChangeSharePermission {
    when: ChangeSharePermission(actor, share, new_permission)

    let actor_share = ResourceShare{resource: share.resource, user: actor}

    requires: actor = share.resource.owner or actor_share.can_admin
    requires: share.user != share.resource.owner    -- can't change owner
    requires: share.status = active

    ensures: share.permission = new_permission
}

rule RevokeShare {
    when: RevokeShare(actor, share)

    let actor_share = ResourceShare{resource: share.resource, user: actor}

    requires: actor = share.resource.owner or actor_share.can_admin
    requires: share.user != share.resource.owner
    requires: share.status = active

    ensures: share.status = revoked
    ensures: Notification.created(
        to: share.user,
        template: access_revoked,
        data: { resource: share.resource }
    )
}

------------------------------------------------------------
-- Surfaces
------------------------------------------------------------

surface ResourceSharing {
    facing sharer: User

    context resource: Resource

    let share = ResourceShare{resource: resource, user: sharer}

    exposes:
        resource.active_shares
        resource.pending_invitations

    provides:
        InviteToResource(sharer, resource, email, permission)
            when sharer = resource.owner or share.can_invite
        for invitation in resource.pending_invitations:
            RevokeInvitation(sharer, invitation)
                when sharer = resource.owner or share.can_admin
        for s in resource.active_shares:
            ChangeSharePermission(sharer, s, new_permission)
                when sharer = resource.owner or share.can_admin
            RevokeShare(sharer, s)
                when sharer = resource.owner or share.can_admin

    @guarantee OwnerCannotBeRevoked
        -- The resource owner's access cannot be revoked or downgraded.
}

surface InvitationResponse {
    facing recipient: User

    context invitation: ResourceInvitation where email = recipient.email

    exposes:
        invitation.resource.name
        invitation.permission
        invitation.invited_by.name
        invitation.expires_at
        invitation.is_valid

    provides:
        ExistingUserAcceptsInvitation(invitation, recipient)
            when invitation.is_valid
        DeclineInvitation(invitation)
            when invitation.is_valid
}
```

**Key language features shown:**
- Named enum (`Permission`) shared across `ResourceShare` and `ResourceInvitation`
- Complex permission logic in `requires`
- Distinct trigger names for different parameter shapes (`ExistingUserAcceptsInvitation` vs `NewUserAcceptsInvitation`)
- Invitation lifecycle (pending → accepted/declined/expired/revoked)
- Checking existence with `exists` keyword
- Permission escalation prevention (`can't invite as admin unless owner`)
- Surfaces for both resource owner and invitation recipient boundaries
- Conditional `provides` with `for` iteration over collections

---

## Pattern 4: Soft Delete & Restore

**Demonstrates:** Simple state machines, projections that filter deleted items, retention policies

This pattern implements soft delete where items appear deleted but can be restored within a retention period.

```
-- allium: 3
-- soft-delete.allium

config {
    retention_period: Duration = 30.days
}

------------------------------------------------------------
-- Entities
------------------------------------------------------------

entity Document {
    workspace: Workspace
    title: String
    content: String
    created_by: User
    created_at: Timestamp
    status: active | deleted
    deleted_at: Timestamp?
    deleted_by: User?

    -- Derived
    is_active: status = active
    retention_expires_at: deleted_at + config.retention_period
    can_restore: status = deleted and retention_expires_at > now
}

-- Extend Workspace to show how projections filter
entity Workspace {
    name: String

    -- Relationships
    all_documents: Document with workspace = this

    -- Projections (what users typically see)
    documents: all_documents where status = active
    deleted_documents: all_documents where status = deleted
    restorable_documents: all_documents where can_restore = true
}

------------------------------------------------------------
-- Rules
------------------------------------------------------------

rule DeleteDocument {
    when: DeleteDocument(actor, document)

    let membership = WorkspaceMembership{user: actor, workspace: document.workspace}

    requires: document.status = active
    requires: actor = document.created_by or membership.can_admin

    ensures: document.status = deleted
    ensures: document.deleted_at = now
    ensures: document.deleted_by = actor
}

rule RestoreDocument {
    when: RestoreDocument(actor, document)

    let membership = WorkspaceMembership{user: actor, workspace: document.workspace}

    requires: document.can_restore
    requires: actor = document.deleted_by or membership.can_admin

    ensures: document.status = active
    ensures: document.deleted_at = null
    ensures: document.deleted_by = null
}

rule PermanentlyDelete {
    when: PermanentlyDelete(actor, document)

    let membership = WorkspaceMembership{user: actor, workspace: document.workspace}

    requires: document.status = deleted
    requires: membership.can_admin

    ensures: not exists document    -- actually removed
}

rule RetentionExpires {
    when: document: Document.retention_expires_at <= now

    requires: document.status = deleted

    ensures: not exists document
}

------------------------------------------------------------
-- Bulk operations
------------------------------------------------------------

rule EmptyTrash {
    when: EmptyTrash(actor, workspace)

    let membership = WorkspaceMembership{user: actor, workspace: workspace}

    requires: membership.can_admin

    ensures:
        for d in workspace.deleted_documents:
            not exists d
}

rule RestoreAll {
    when: RestoreAllDeleted(actor, workspace)

    let membership = WorkspaceMembership{user: actor, workspace: workspace}

    requires: membership.can_admin

    ensures:
        for d in workspace.restorable_documents:
            d.status = active
            d.deleted_at = null
            d.deleted_by = null
}
```

**Key language features shown:**
- `status` field with clear lifecycle
- Nullable timestamps (`deleted_at: Timestamp?`)
- Projections filtering by status (`documents: all_documents where status = active`)
- Derived values using config (`retention_expires_at: deleted_at + config.retention_period`)
- Temporal trigger for automatic cleanup (`when: document: Document.retention_expires_at <= now`)
- `not exists` for permanent removal, as distinct from soft delete
- Bulk operations with `for` iteration

---

## Pattern 5: Notification Preferences & Digests

**Demonstrates:** Sum types for notification variants, user preferences affecting rule behaviour, digest batching, temporal triggers, surfaces

This pattern handles in-app notifications with user-controlled email preferences and digest batching. It uses sum types to model different notification kinds, each carrying its own contextual data rather than pre-computed strings.

```
-- allium: 3
-- notifications.allium
-- Elided types: Comment, Resource, Task, Permission, DayOfWeek
-- (defined in other patterns or your domain spec)

config {
    digest_window: Duration = 24.hours
}

------------------------------------------------------------
-- Enumerations
------------------------------------------------------------

enum EmailFrequency { immediately | daily_digest | never }

------------------------------------------------------------
-- Entities
------------------------------------------------------------

entity User {
    email: String
    name: String
    next_digest_at: Timestamp?

    -- Relationships
    notification_setting: NotificationSetting with user = this
    notifications: Notification with user = this

    -- Projections
    unread_notifications: notifications where status = unread
    pending_email_notifications: notifications where email_status = pending
    recent_pending_notifications: notifications where email_status = pending and created_at >= now - config.digest_window
}

entity NotificationSetting {
    user: User

    -- Per-type email preferences
    email_on_mention: EmailFrequency
    email_on_comment: EmailFrequency
    email_on_share: EmailFrequency
    email_on_assignment: EmailFrequency

    -- Global settings
    digest_enabled: Boolean
    digest_day_of_week: Set<DayOfWeek>    -- domain type; define as enum in your spec
}

------------------------------------------------------------
-- Notification Sum Type
------------------------------------------------------------

-- Base notification entity with shared fields
entity Notification {
    user: User
    created_at: Timestamp
    status: unread | read | archived
    email_status: pending | sent | skipped | digested
    kind: MentionNotification | ReplyNotification | ShareNotification |
          AssignmentNotification | SystemNotification

    -- Derived
    is_unread: status = unread
}

-- Someone @mentioned the user in a comment
variant MentionNotification : Notification {
    comment: Comment
    mentioned_by: User
}

-- Someone replied to the user's comment
variant ReplyNotification : Notification {
    reply: Comment              -- the new reply
    original_comment: Comment   -- the user's comment being replied to
    replied_by: User
}

-- Someone shared a resource with the user
variant ShareNotification : Notification {
    resource: Resource
    shared_by: User
    permission: Permission
}

-- Someone assigned a task to the user
variant AssignmentNotification : Notification {
    task: Task
    assigned_by: User
}

-- System-generated notification (catch-all for non-structured notifications)
variant SystemNotification : Notification {
    title: String
    body: String
    link: String?
}

------------------------------------------------------------
-- Supporting Entities
------------------------------------------------------------

entity DigestBatch {
    user: User
    notifications: Set<Notification>
    created_at: Timestamp
    sent_at: Timestamp?
    status: pending | sent | failed
}

------------------------------------------------------------
-- Creating notifications (type-specific rules)
------------------------------------------------------------

rule CreateMentionNotification {
    when: UserMentioned(user, comment, mentioned_by)

    let settings = user.notification_setting

    requires: user != mentioned_by    -- don't notify self

    ensures: MentionNotification.created(
        user: user,
        comment: comment,
        mentioned_by: mentioned_by,
        created_at: now,
        status: unread,
        email_status: if settings.email_on_mention = never: skipped else: pending
    )
}

rule CreateReplyNotification {
    when: CommentReplied(original_author, reply, original_comment)

    let settings = original_author.notification_setting

    requires: original_author != reply.author    -- don't notify self

    ensures: ReplyNotification.created(
        user: original_author,
        reply: reply,
        original_comment: original_comment,
        replied_by: reply.author,
        created_at: now,
        status: unread,
        email_status: if settings.email_on_comment = never: skipped else: pending
    )
}

rule CreateShareNotification {
    when: ResourceShared(user, resource, shared_by, permission)

    let settings = user.notification_setting

    requires: user != shared_by    -- don't notify self

    ensures: ShareNotification.created(
        user: user,
        resource: resource,
        shared_by: shared_by,
        permission: permission,
        created_at: now,
        status: unread,
        email_status: if settings.email_on_share = never: skipped else: pending
    )
}

rule CreateAssignmentNotification {
    when: TaskAssigned(user, task, assigned_by)

    let settings = user.notification_setting

    requires: user != assigned_by    -- don't notify self

    ensures: AssignmentNotification.created(
        user: user,
        task: task,
        assigned_by: assigned_by,
        created_at: now,
        status: unread,
        email_status: if settings.email_on_assignment = never: skipped else: pending
    )
}

rule CreateSystemNotification {
    when: SystemNotificationTriggered(user, title, body, link)

    ensures: SystemNotification.created(
        user: user,
        title: title,
        body: body,
        link: link,
        created_at: now,
        status: unread,
        email_status: pending
    )
}

------------------------------------------------------------
-- Immediate email sending
------------------------------------------------------------

rule SendImmediateEmail {
    when: notification: Notification.created

    let settings = notification.user.notification_setting
    let preference =
        if notification.kind = MentionNotification: settings.email_on_mention
        else if notification.kind = ReplyNotification: settings.email_on_comment
        else if notification.kind = ShareNotification: settings.email_on_share
        else if notification.kind = AssignmentNotification: settings.email_on_assignment
        else: immediately    -- system notifications send immediately by default

    requires: notification.email_status = pending
    requires: preference = immediately

    ensures: Email.created(
        to: notification.user.email,
        template: notification_immediate,
        data: { notification: notification }
    )
    ensures: notification.email_status = sent
}

------------------------------------------------------------
-- Reading notifications
------------------------------------------------------------

rule MarkAsRead {
    when: MarkNotificationRead(user, notification)

    requires: notification.user = user
    requires: notification.status = unread

    ensures: notification.status = read
}

rule MarkAllAsRead {
    when: MarkAllNotificationsRead(user)

    ensures:
        for n in user.unread_notifications:
            n.status = read
}

rule ArchiveNotification {
    when: ArchiveNotification(user, notification)

    requires: notification.user = user

    ensures: notification.status = archived
}

------------------------------------------------------------
-- Daily digest
------------------------------------------------------------

rule CreateDailyDigest {
    when: user: User.next_digest_at <= now

    requires: user.notification_setting.digest_enabled

    let pending = user.recent_pending_notifications

    requires: pending.count > 0

    ensures: DigestBatch.created(
        user: user,
        notifications: pending,
        created_at: now,
        status: pending
    )
    ensures:
        for n in pending:
            n.email_status = digested
    ensures: user.next_digest_at = next_digest_time(user)    -- black box; uses digest_day_of_week
}

rule SendDigest {
    when: batch: DigestBatch.created

    requires: batch.status = pending
    requires: batch.notifications.count > 0

    ensures: Email.created(
        to: batch.user.email,
        template: daily_digest,
        data: {
            notifications: batch.notifications,
            unread_count: batch.user.unread_notifications.count
        }
    )
    ensures: batch.status = sent
    ensures: batch.sent_at = now
}

------------------------------------------------------------
-- Preference updates
------------------------------------------------------------

rule UpdateNotificationPreferences {
    when: UpdatePreferences(user, preferences)

    let settings = user.notification_setting

    ensures: settings.email_on_mention = preferences.mention
    ensures: settings.email_on_comment = preferences.comment
    ensures: settings.email_on_share = preferences.share
    ensures: settings.email_on_assignment = preferences.assignment
    ensures: settings.digest_enabled = preferences.digest_enabled
    ensures: settings.digest_day_of_week = preferences.digest_days
}

------------------------------------------------------------
-- Surfaces
------------------------------------------------------------

surface NotificationCentre {
    facing user: User

    exposes:
        user.unread_notifications
        user.unread_notifications.count
        user.notifications

    provides:
        for notification in user.unread_notifications:
            MarkNotificationRead(user, notification)
        MarkAllNotificationsRead(user)
            when user.unread_notifications.count > 0
        for notification in user.notifications:
            ArchiveNotification(user, notification)

    related:
        NotificationPreferences(user)
}

surface NotificationPreferences {
    facing user: User

    context settings: NotificationSetting where user = user

    exposes:
        settings.email_on_mention
        settings.email_on_comment
        settings.email_on_share
        settings.email_on_assignment
        settings.digest_enabled
        settings.digest_day_of_week

    provides:
        UpdatePreferences(user, preferences)
}
```

**Key language features shown:**
- **Sum types**: `kind: MentionNotification | ReplyNotification | ...` declares notification variants
- **Variant declarations**: Each notification kind uses `variant X : Notification` syntax
- **Variant-specific creation rules**: Each variant has its own creation rule with appropriate fields
- **Exhaustive kind checking**: `SendImmediateEmail` handles all variants explicitly
- Named enum (`EmailFrequency`) shared across preference fields
- User preferences stored as entity
- Temporal trigger for per-user digest scheduling (`when: user: User.next_digest_at <= now`)
- Digest batching with temporal trigger
- Surfaces with `related` clause linking notification centre to preferences

**Why sum types here?**

The previous approach used pre-computed `title`, `body`, and `link` strings:
```
Notification.created(
    type: mention,
    title: "{author} mentioned you",
    body: truncate(comment.body, 100),
    link: comment.parent.url
)
```

With sum types, each notification carries its actual entity references:
```
MentionNotification.created(
    comment: comment,
    mentioned_by: author
)
```

This is better because:
1. **Rich queries**: "Show all notifications about this document" queries the actual relationships
2. **Type safety**: Creating a `MentionNotification` requires a `comment` - you can't forget it
3. **Flexible rendering**: Display logic can access full entity data, not just truncated strings
4. **Consistency**: If a user's name changes, notification titles reflect the current name

---

## Pattern 6: Usage Limits & Quotas

**Demonstrates:** Limit checks in `requires`, metered resources, plan tiers, overage handling, surfaces

This pattern handles SaaS usage limits: different plans have different quotas, and usage is tracked and enforced.

```
-- allium: 3
-- usage-limits.allium
-- Elided types: Feature (define as enum in your spec)

------------------------------------------------------------
-- Entities
------------------------------------------------------------

entity Plan {
    name: String                    -- e.g., "free", "pro", "enterprise"

    -- Limits (null = unlimited)
    max_documents: Integer?
    max_storage_bytes: Integer?
    max_team_members: Integer?
    max_api_requests_per_day: Integer?

    -- Features
    features: Set<Feature>          -- domain type; define in your spec

    -- Derived
    has_unlimited_documents: max_documents = null
    has_unlimited_storage: max_storage_bytes = null
    has_unlimited_members: max_team_members = null
}

entity Workspace {
    name: String
    owner: User
    plan: Plan
    api_key: String?

    -- Relationships
    documents: Document with workspace = this
    memberships: WorkspaceMembership with workspace = this
    usage: WorkspaceUsage with workspace = this

    -- Derived checks
    can_add_document: plan.has_unlimited_documents or documents.count < plan.max_documents
    can_add_member: plan.has_unlimited_members or memberships.count < plan.max_team_members
    can_use_feature(f): f in plan.features
}

entity WorkspaceUsage {
    workspace: Workspace
    storage_bytes_used: Integer
    api_requests_today: Integer
    next_reset_at: Timestamp

    -- Derived (null when plan has no limit)
    api_requests_remaining:
        workspace.plan.max_api_requests_per_day - api_requests_today
    has_api_quota: workspace.plan.max_api_requests_per_day != null
    is_over_api_quota: has_api_quota and api_requests_remaining <= 0
}

entity UsageEvent {
    workspace: Workspace
    type: document_created | document_deleted | storage_added |
          storage_removed | api_request | member_added | member_removed
    amount: Integer
    recorded_at: Timestamp
}

------------------------------------------------------------
-- Defaults
------------------------------------------------------------

default Plan free = {
    name: "free",
    max_documents: 10,
    max_storage_bytes: 100_000_000,    -- 100MB
    max_team_members: 3,
    max_api_requests_per_day: 100,
    features: { basic_editing }
}

default Plan pro = {
    name: "pro",
    max_documents: 1000,
    max_storage_bytes: 10_000_000_000,  -- 10GB
    max_team_members: 20,
    max_api_requests_per_day: 10000,
    features: { basic_editing, advanced_editing, api_access, integrations }
}

default Plan enterprise = {
    name: "enterprise",
    features: { basic_editing, advanced_editing, api_access, integrations,
                sso, audit_log, custom_branding }
    -- max_documents, max_storage_bytes, max_team_members,
    -- max_api_requests_per_day all null (unlimited)
}

------------------------------------------------------------
-- Enforcing limits
------------------------------------------------------------

rule CreateDocument {
    when: CreateDocument(user, workspace, title)

    requires: workspace.can_add_document

    ensures: Document.created(workspace: workspace, title: title, created_by: user)
    ensures: UsageEvent.created(
        workspace: workspace,
        type: document_created,
        amount: 1,
        recorded_at: now
    )
}

rule CreateDocumentLimitReached {
    when: CreateDocument(user, workspace, title)

    requires: not workspace.can_add_document

    ensures: UserInformed(
        user: user,
        about: limit_reached,
        data: {
            limit_type: documents,
            current: workspace.documents.count,
            max: workspace.plan.max_documents,
            upgrade_path: next_plan(workspace.plan)
        }
    )
}

rule AddTeamMember {
    when: AddMember(actor, workspace, new_member, role)

    requires: workspace.can_add_member
    requires: WorkspaceMembership{user: actor, workspace: workspace}.can_admin

    ensures: WorkspaceMembership.created(...)
    ensures: UsageEvent.created(
        workspace: workspace,
        type: member_added,
        amount: 1,
        recorded_at: now
    )
}

rule UseFeature {
    when: UseFeature(user, workspace, feature)

    requires: workspace.can_use_feature(feature)

    ensures: FeatureUsed(workspace: workspace, feature: feature, by: user)
}

rule UseFeatureNotAvailable {
    when: UseFeature(user, workspace, feature)

    requires: not workspace.can_use_feature(feature)

    ensures: UserInformed(
        user: user,
        about: feature_not_available,
        data: {
            feature: feature,
            available_on: plans_with_feature(feature)
        }
    )
}

------------------------------------------------------------
-- API rate limiting
------------------------------------------------------------

rule RecordApiRequest {
    when: ApiRequestReceived(workspace, endpoint)

    let usage = workspace.usage

    requires: not usage.is_over_api_quota

    ensures: usage.api_requests_today = usage.api_requests_today + 1
    ensures: UsageEvent.created(
        workspace: workspace,
        type: api_request,
        amount: 1,
        recorded_at: now
    )
}

rule ApiRateLimitExceeded {
    when: ApiRequestReceived(workspace, endpoint)

    let usage = workspace.usage

    requires: usage.is_over_api_quota

    ensures: ApiRequestRejected(
        workspace: workspace,
        reason: rate_limit_exceeded,
        data: { resets_at: usage.next_reset_at }
    )
}

rule ResetDailyApiUsage {
    when: usage: WorkspaceUsage.next_reset_at <= now

    requires: usage.api_requests_today > 0    -- prevents re-firing when already reset

    ensures: usage.api_requests_today = 0
    ensures: usage.next_reset_at = usage.next_reset_at + 1.day
}

------------------------------------------------------------
-- Plan changes
------------------------------------------------------------

rule UpgradePlan {
    when: UpgradePlan(workspace, new_plan)

    let old_plan = workspace.plan

    requires: new_plan.max_documents >= old_plan.max_documents
              or new_plan.has_unlimited_documents

    ensures: workspace.plan = new_plan
    ensures: Email.created(
        to: workspace.owner.email,
        template: plan_upgraded,
        data: { old_plan: old_plan, new_plan: new_plan }
    )
}

rule DowngradePlan {
    when: DowngradePlan(workspace, new_plan)

    let old_plan = workspace.plan

    -- Can only downgrade if under new plan's limits
    requires: workspace.documents.count <= new_plan.max_documents
              or new_plan.has_unlimited_documents
    requires: workspace.memberships.count <= new_plan.max_team_members
              or new_plan.has_unlimited_members
    requires: workspace.usage.storage_bytes_used <= new_plan.max_storage_bytes
              or new_plan.has_unlimited_storage

    ensures: workspace.plan = new_plan
    ensures: Email.created(
        to: workspace.owner.email,
        template: plan_downgraded,
        data: { old_plan: old_plan, new_plan: new_plan }
    )
}

rule DowngradeBlocked {
    when: DowngradePlan(workspace, new_plan)

    let over_documents =
        workspace.documents.count > new_plan.max_documents
        and not new_plan.has_unlimited_documents
    let over_members =
        workspace.memberships.count > new_plan.max_team_members
        and not new_plan.has_unlimited_members
    let over_storage =
        workspace.usage.storage_bytes_used > new_plan.max_storage_bytes
        and not new_plan.has_unlimited_storage

    requires: over_documents or over_members or over_storage

    ensures: UserInformed(
        user: workspace.owner,
        about: downgrade_blocked,
        data: {
            over_documents: over_documents,
            over_members: over_members,
            over_storage: over_storage
        }
    )
}

------------------------------------------------------------
-- Actors
------------------------------------------------------------

actor WorkspaceOwner {
    within: Workspace
    identified_by: User where this = within.owner
}

------------------------------------------------------------
-- Surfaces
------------------------------------------------------------

surface UsageDashboard {
    facing owner: WorkspaceOwner

    context workspace: Workspace

    exposes:
        workspace.plan
        workspace.documents.count
        workspace.plan.max_documents
        workspace.memberships.count
        workspace.plan.max_team_members
        workspace.usage.storage_bytes_used
        workspace.plan.max_storage_bytes
        workspace.usage.api_requests_today
        workspace.usage.api_requests_remaining

    provides:
        UpgradePlan(workspace, new_plan)
        DowngradePlan(workspace, new_plan)

    @guidance
        -- Show progress bars for usage against limits.
        -- Highlight when any resource is above 80% of its limit.
}

surface APIAccess {
    facing consumer: Workspace

    exposes:
        consumer.usage.api_requests_remaining
        consumer.plan.max_api_requests_per_day

    provides:
        ApiRequestReceived(consumer, endpoint)
            when not consumer.usage.is_over_api_quota

    @guarantee RateLimitEnforcement
        -- Requests beyond the daily limit receive HTTP 429 with
        -- reset time.
}
```

**Key language features shown:**
- Plan definitions with limits
- Derived boolean checks for limit enforcement (`can_add_document`, `can_add_member`)
- `requires` checking limits before actions
- Paired rules for success/failure cases
- Usage tracking with events
- Temporal trigger for daily reset (`when: usage: WorkspaceUsage.next_reset_at <= now`)
- Plan upgrade/downgrade logic with `let` binding to capture pre-mutation state
- Feature flags (`can_use_feature(f)`)
- Interaction surface for usage dashboard and API surface with rate limit guarantee

---

## Pattern 7: Comments with Mentions

**Demonstrates:** Nested entities, parsing for mentions, cross-entity notifications, threading, surfaces

This pattern implements comments with @mentions, including mention parsing and notification generation.

```
-- allium: 3
-- comments.allium

------------------------------------------------------------
-- Entities
------------------------------------------------------------

external entity User {
    name: String
    is_admin: Boolean
}

external entity Commentable {
    -- Defined by the consuming spec (e.g., Document, Task, Project)
}

entity Comment {
    parent: Commentable
    reply_to: Comment?              -- null for top-level, set for replies
    author: User
    body: String
    created_at: Timestamp
    edited_at: Timestamp?
    status: active | deleted

    -- Relationships
    mentions: CommentMention with comment = this
    replies: Comment with reply_to = this
    reactions: CommentReaction with comment = this

    -- Projections
    active_replies: replies where status = active

    -- Derived
    is_reply: reply_to != null
    is_edited: edited_at != null
    mentioned_users: mentions -> user
    thread_depth: if is_reply: reply_to.thread_depth + 1 else: 0
}

-- Join entity for mentions
entity CommentMention {
    comment: Comment
    user: User
    notified: Boolean
}

entity CommentReaction {
    comment: Comment
    user: User
    emoji: String                   -- e.g., "👍", "❤️", "🎉"
    created_at: Timestamp
}

------------------------------------------------------------
-- Creating comments
------------------------------------------------------------

rule CreateComment {
    when: CreateComment(author, parent, body)

    let mentioned_usernames = parse_mentions(body)    -- black box: extracts @username
    let mentioned_users = users_with_usernames(mentioned_usernames)    -- black box lookup

    ensures:
        let comment = Comment.created(
            parent: parent,
            reply_to: null,
            author: author,
            body: body,
            created_at: now,
            status: active
        )
        for user in mentioned_users:
            CommentMention.created(
                comment: comment,
                user: user,
                notified: false
            )
}

rule CreateReply {
    when: CreateReply(author, parent_comment, body)

    let mentioned_usernames = parse_mentions(body)
    let mentioned_users = users_with_usernames(mentioned_usernames)    -- black box lookup

    requires: parent_comment.status = active
    requires: parent_comment.thread_depth < 3    -- limit nesting

    ensures:
        let comment = Comment.created(
            parent: parent_comment.parent,
            reply_to: parent_comment,
            author: author,
            body: body,
            created_at: now,
            status: active
        )
        for user in mentioned_users:
            CommentMention.created(
                comment: comment,
                user: user,
                notified: false
            )
}

------------------------------------------------------------
-- Notifications for mentions and replies
------------------------------------------------------------

-- Trigger the notification system when someone is mentioned
rule NotifyMentionedUser {
    when: mention: CommentMention.created

    requires: mention.user != mention.comment.author    -- don't notify self
    requires: not mention.notified

    ensures: mention.notified = true
    ensures: UserMentioned(
        user: mention.user,
        comment: mention.comment,
        mentioned_by: mention.comment.author
    )
}

-- Trigger the notification system when someone's comment receives a reply
rule NotifyCommentAuthorOfReply {
    when: comment: Comment.created

    let original_author = comment.reply_to?.author

    requires: comment.is_reply
    requires: original_author != null
    requires: original_author != comment.author    -- don't notify self
    requires: original_author not in comment.mentioned_users    -- avoid double notify

    ensures: CommentReplied(
        original_author: original_author,
        reply: comment,
        original_comment: comment.reply_to
    )
}

------------------------------------------------------------
-- Editing
------------------------------------------------------------

rule EditComment {
    when: EditComment(actor, comment, new_body)

    requires: actor = comment.author
    requires: comment.status = active

    let old_mentions = comment.mentioned_users
    let new_mentioned_usernames = parse_mentions(new_body)
    let new_mentioned_users = users_with_usernames(new_mentioned_usernames)    -- black box lookup
    let added_mentions = new_mentioned_users - old_mentions
    let removed_mentions = old_mentions - new_mentioned_users

    ensures: comment.body = new_body
    ensures: comment.edited_at = now

    -- Remove old mentions that are no longer present
    ensures:
        for user in removed_mentions:
            not exists CommentMention{comment, user}

    -- Add new mentions
    ensures:
        for user in added_mentions:
            CommentMention.created(
                comment: comment,
                user: user,
                notified: false
            )
}

------------------------------------------------------------
-- Deleting
------------------------------------------------------------

rule DeleteComment {
    when: DeleteComment(actor, comment)

    requires: actor = comment.author or actor.is_admin
    requires: comment.status = active

    ensures: comment.status = deleted
    -- Note: replies remain but show "deleted comment"
}

------------------------------------------------------------
-- Reactions
------------------------------------------------------------

rule AddReaction {
    when: AddReaction(user, comment, emoji)

    requires: comment.status = active
    requires: not exists CommentReaction{comment, user, emoji}

    ensures: CommentReaction.created(
        comment: comment,
        user: user,
        emoji: emoji,
        created_at: now
    )
}

rule RemoveReaction {
    when: RemoveReaction(user, comment, emoji)

    let reaction = CommentReaction{comment, user, emoji}

    requires: comment.status = active
    requires: exists reaction

    ensures: not exists reaction
}

rule ToggleReaction {
    when: ToggleReaction(user, comment, emoji)

    let existing = CommentReaction{comment, user, emoji}

    requires: comment.status = active

    ensures:
        if exists existing:
            not exists existing
        else:
            CommentReaction.created(
                comment: comment,
                user: user,
                emoji: emoji,
                created_at: now
            )
}

------------------------------------------------------------
-- Surfaces
------------------------------------------------------------

surface CommentThread {
    facing viewer: User

    context parent: Commentable

    let comments = Comments where parent = parent and status = active

    exposes:
        for comment in comments:
            comment.author.name
            comment.body
            comment.created_at
            comment.is_edited
            comment.active_replies
            comment.reactions

    provides:
        CreateComment(viewer, parent, body)
        for comment in comments:
            CreateReply(viewer, comment, body)
                when comment.thread_depth < 3
            EditComment(viewer, comment, new_body)
                when viewer = comment.author
            DeleteComment(viewer, comment)
                when viewer = comment.author or viewer.is_admin
            AddReaction(viewer, comment, emoji)
            RemoveReaction(viewer, comment, emoji)
                when exists CommentReaction{comment: comment, user: viewer, emoji: emoji}

    @guidance
        -- Show "edited" indicator when comment.is_edited.
        -- Show "deleted comment" placeholder for deleted replies
        -- rather than removing them from the thread.
}
```

**Key language features shown:**
- Nested/recursive entities (comments with replies)
- Entity creation triggers with binding (`when: mention: CommentMention.created`)
- Black box functions (`parse_mentions()`, `users_with_usernames()`)
- Explicit `let` binding for created entities
- Set operations (`new_mentioned_users - old_mentions`)
- Depth limiting (`thread_depth < 3`)
- **Cross-pattern triggers**: Emits `UserMentioned` and `CommentReplied` triggers that Pattern 5 handles
- Avoiding double notifications (`original_author not in comment.mentioned_users`)
- Toggle pattern with conditional ensures
- Join entity with three keys (`CommentReaction{comment, user, emoji}`)
- Surface with role-conditional actions (author can edit, author or admin can delete)

---

## Pattern 8: Integrating Library Specs

**Demonstrates:** External spec references with coordinates, configuration blocks, config parameter references, responding to external triggers, using external entities

Library specs are standalone specifications for common functionality: authentication providers, payment processors, email services. They define a contract that implementations must satisfy, and your application spec composes them in. Consuming specs can reference a library spec's config values as defaults for their own parameters, avoiding duplication when the values should track each other.

### Example: OAuth Authentication

This example shows integrating a library OAuth spec into your application. The OAuth spec handles the authentication flow; your application responds to authentication events and manages application-level user state.

```
-- allium: 3
-- app-auth.allium

------------------------------------------------------------
-- External Spec References
------------------------------------------------------------

-- Reference the OAuth spec from the library
-- The coordinate is immutable (git SHA), ensuring reproducible specs
use "github.com/allium-specs/oauth2/af8e2c1d" as oauth

-- Configure the OAuth spec for our application
oauth/config {
    providers: { google, microsoft, github }
    session_duration: 24.hours
    refresh_window: 1.hour
    link_expiry: 15.minutes
}

------------------------------------------------------------
-- Application Entities
------------------------------------------------------------

-- Our application's User entity, linked to OAuth identities
entity User {
    email: String
    name: String
    avatar_url: String?
    status: active | suspended | deactivated
    created_at: Timestamp
    last_login_at: Timestamp?

    -- Relationship to OAuth sessions (from external spec)
    sessions: oauth/Session with user = this
    identities: oauth/Identity with user = this

    -- Projections
    active_sessions: sessions where status = active

    -- Derived
    is_authenticated: active_sessions.count > 0
    linked_providers: identities -> provider
}

-- Application-specific user preferences
entity UserPreferences {
    user: User
    theme: light | dark | system
    timezone: String
    locale: String
}

------------------------------------------------------------
-- Responding to OAuth Events
------------------------------------------------------------

-- When a user authenticates for the first time, create our User entity
rule CreateUserOnFirstLogin {
    when: oauth/AuthenticationSucceeded(identity, session)

    requires: not exists User{email: identity.email}

    ensures:
        let user = User.created(
            email: identity.email,
            name: identity.display_name,
            avatar_url: identity.avatar_url,
            status: active,
            created_at: now,
            last_login_at: now
        )
        -- Link the OAuth identity to our user
        identity.user = user
        session.user = user
        -- Create default preferences
        UserPreferences.created(
            user: user,
            theme: system,
            timezone: identity.timezone ?? "UTC",
            locale: identity.locale ?? "en"
        )
        Email.created(
            to: user.email,
            template: welcome,
            data: { user: user, provider: identity.provider }
        )
}

-- When an existing user logs in, update last login
rule UpdateUserOnLogin {
    when: oauth/AuthenticationSucceeded(identity, session)

    let user = User{email: identity.email}

    requires: exists user
    requires: user.status = active

    ensures: user.last_login_at = now
    ensures: session.user = user
}

-- Block login for suspended users
rule BlockSuspendedUserLogin {
    when: oauth/AuthenticationSucceeded(identity, session)

    let user = User{email: identity.email}

    requires: exists user
    requires: user.status = suspended

    ensures: session.status = revoked
    ensures: UserInformed(
        user: user,
        about: account_suspended,
        data: { contact: "support@example.com" }
    )
}

-- When OAuth session expires, we might want to notify
rule NotifySessionExpiring {
    when: session: oauth/Session.status transitions_to expiring

    let user = session.user

    requires: user != null

    ensures: UserInformed(
        user: user,
        about: session_expiring,
        data: { time_remaining: session.time_remaining }
    )
}

-- Audit logging for security events
rule AuditLogout {
    when: oauth/SessionTerminated(session, reason)

    let user = session.user

    requires: user != null

    ensures: AuditLog.created(
        user: user,
        event: logout,
        reason: reason,
        timestamp: now,
        metadata: { provider: session.provider, session_start: session.created_at }
    )
}

------------------------------------------------------------
-- Application Actions Using OAuth
------------------------------------------------------------

rule LinkAdditionalProvider {
    when: LinkProvider(user, provider)

    requires: user.status = active
    requires: provider not in user.linked_providers

    -- Trigger the OAuth flow from the library spec
    ensures: oauth/InitiateAuthentication(
        provider: provider,
        intent: link_account,
        existing_user: user
    )
}

rule UnlinkProvider {
    when: UnlinkProvider(user, provider)

    let identity = oauth/Identity{user, provider}

    requires: user.status = active
    requires: exists identity
    requires: user.linked_providers.count > 1    -- must keep at least one

    ensures: not exists identity
    ensures: AuditLog.created(
        user: user,
        event: provider_unlinked,
        timestamp: now,
        metadata: { provider: provider }
    )
}
```

### Example: Payment Processing

This example shows integrating a payment processor spec for subscription billing.

```
-- allium: 3
-- billing.allium

------------------------------------------------------------
-- External Spec References
------------------------------------------------------------

use "github.com/allium-specs/stripe-billing/b2c4e6f8" as stripe

stripe/config {
    currency: USD
    tax_calculation: automatic
    proration: create_prorations
    trial_period: 14.days
}

config {
    trial_period: Duration = stripe/config.trial_period
    extended_trial: Duration = stripe/config.trial_period * 2
    trial_reminder_lead: Duration = 3.days
}

------------------------------------------------------------
-- Application Entities
------------------------------------------------------------

entity Organisation {
    name: String
    owner: User
    billing_portal_url: String?

    -- Link to Stripe customer (from external spec)
    stripe_customer: stripe/Customer?

    -- Relationships
    subscription: Subscription with organisation = this
    invoices: stripe/Invoice with stripe_customer = this

    -- Derived
    is_paying: subscription?.status = active
    has_payment_method: stripe_customer?.default_payment_method != null
}

entity Subscription {
    organisation: Organisation
    plan: Plan
    status: trialing | active | past_due | cancelled | expired
    started_at: Timestamp
    trial_ends_at: Timestamp?
    current_period_ends_at: Timestamp
    trial_reminder_sent: Boolean

    -- Link to Stripe subscription
    stripe_subscription: stripe/Subscription?

    -- Derived
    is_trial: status = trialing
    days_until_renewal: current_period_ends_at - now
}

------------------------------------------------------------
-- Responding to Payment Events
------------------------------------------------------------

-- When Stripe confirms payment, activate or renew subscription
rule ActivateOnPaymentSuccess {
    when: stripe/PaymentSucceeded(invoice)

    let customer = invoice.customer
    let org = Organisation{stripe_customer: customer}
    let sub = org.subscription

    requires: exists org
    requires: sub.status in {trialing, past_due}

    ensures: sub.status = active
    ensures: sub.current_period_ends_at = invoice.period_end
    ensures: Email.created(
        to: org.owner.email,
        template: payment_confirmed,
        data: { amount: invoice.amount, next_billing: invoice.period_end }
    )
}

-- Handle failed payments
rule HandlePaymentFailure {
    when: stripe/PaymentFailed(invoice, failure_reason)

    let customer = invoice.customer
    let org = Organisation{stripe_customer: customer}
    let sub = org.subscription

    requires: exists org

    ensures: sub.status = past_due
    ensures: Email.created(
        to: org.owner.email,
        template: payment_failed,
        data: {
            reason: failure_reason,
            retry_date: invoice.next_payment_attempt,
            update_payment_url: org.billing_portal_url
        }
    )
    ensures: UserInformed(
        user: org.owner,
        about: payment_failed,
        data: { reason: failure_reason }
    )
}

-- When trial is ending, remind user
rule TrialEndingReminder {
    when: sub: Subscription.trial_ends_at - config.trial_reminder_lead <= now

    requires: sub.status = trialing
    requires: not sub.trial_reminder_sent

    let org = sub.organisation

    ensures: sub.trial_reminder_sent = true
    ensures: Email.created(
        to: org.owner.email,
        template: trial_ending,
        data: {
            days_remaining: config.trial_reminder_lead,
            plan: sub.plan,
            has_payment_method: org.has_payment_method
        }
    )
}

-- Respond to subscription cancellation from Stripe
rule HandleSubscriptionCancelled {
    when: stripe/SubscriptionCancelled(stripe_sub, reason)

    let sub = Subscription{stripe_subscription: stripe_sub}
    let org = sub.organisation

    requires: exists sub

    ensures: sub.status = cancelled
    ensures: Email.created(
        to: org.owner.email,
        template: subscription_cancelled,
        data: { reason: reason, access_until: sub.current_period_ends_at }
    )
    ensures: AuditLog.created(
        user: org.owner,
        event: subscription_cancelled,
        timestamp: now,
        metadata: { reason: reason, plan: sub.plan.name }
    )
}

------------------------------------------------------------
-- Application Actions Using Stripe
------------------------------------------------------------

rule StartSubscription {
    when: StartSubscription(org, plan)

    requires: org.subscription = null or org.subscription.status in {cancelled, expired}
    requires: org.stripe_customer != null
    requires: org.has_payment_method

    ensures: stripe/CreateSubscription(
        customer: org.stripe_customer,
        price: plan.stripe_price_id,
        trial_period: if plan.has_trial: stripe/config.trial_period else: null
    )
}

rule ChangePlan {
    when: ChangePlan(org, new_plan)

    let sub = org.subscription

    requires: sub.status = active
    requires: new_plan != sub.plan

    ensures: stripe/UpdateSubscription(
        subscription: sub.stripe_subscription,
        new_price: new_plan.stripe_price_id
    )
    ensures: sub.plan = new_plan
}

rule CancelSubscription {
    when: CancelSubscription(org, reason)

    let sub = org.subscription

    requires: sub.status in {active, trialing}

    ensures: stripe/CancelSubscription(
        subscription: sub.stripe_subscription,
        at_period_end: true    -- access continues until paid period ends
    )
    ensures: AuditLog.created(
        user: org.owner,
        event: cancellation_requested,
        timestamp: now,
        metadata: { reason: reason }
    )
}
```

**Key language features shown:**
- External spec references with immutable coordinates (`use "github.com/.../abc123" as alias`)
- Configuration blocks for external specs (`oauth/config { ... }`)
- Config parameter references as defaults (`trial_period: Duration = stripe/config.trial_period`)
- Expression-form defaults derived from library config (`extended_trial: Duration = stripe/config.trial_period * 2`)
- Responding to external triggers (`when: oauth/AuthenticationSucceeded(...)`)
- Trigger emissions for cross-pattern notification (`UserInformed(...)`)
- Responding to external state transitions (`when: session: oauth/Session.status transitions_to expiring`)
- Using external entities (`oauth/Session`, `stripe/Customer`)
- Linking application entities to external entities (`stripe_customer: stripe/Customer?`)
- Triggering external actions (`ensures: stripe/CreateSubscription(...)`)
- Qualified names throughout (`oauth/Session`, `stripe/config.trial_period`)

### Library Spec Design Principles

When creating or choosing library specs:

1. **Immutable coordinates**: Always use content-addressed references (git SHAs), never floating versions
2. **Configuration over convention**: Library specs should expose configuration for anything that might vary between applications
3. **Observable triggers**: Library specs should emit triggers for all significant events so consuming specs can respond
4. **Minimal coupling**: Library specs shouldn't depend on your application entities - the linkage goes one way
5. **Clear boundaries**: The library spec handles its domain (OAuth flow, payment processing); your spec handles application concerns (user creation, access control)

---

## Pattern 9: Framework Integration Contract

**Demonstrates:** Contract declarations, expression-bearing invariants, `contracts:` clause with `demands`/`fulfils`, programmatic surfaces, typed signatures

This pattern specifies the contract between an event-sourcing framework and its domain modules. The framework demands that each module supply a deterministic evaluation function; in return, the surface fulfils event submission and state snapshot services. Unlike user-facing surfaces that use `exposes` and `provides`, framework-to-module boundaries use a `contracts:` clause with `demands` and `fulfils` to describe programmatic obligations. Contracts are declared at module level so they can be reused across surfaces or referenced from other specs.

```
-- allium: 3
-- event-sourcing-integration.allium

------------------------------------------------------------
-- Value Types
------------------------------------------------------------

value EntityKey {
    kind: String
    id: String
}

value EventOutcome {
    entity_key: EntityKey
    new_state: ByteArray
    side_effects: List<SideEffect>
}

value SideEffect {
    kind: emit_event | schedule_timeout | request_snapshot
    payload: ByteArray
}

value SnapshotRequest {
    entity_key: EntityKey
    as_of: Timestamp
}

value Snapshot {
    entity_key: EntityKey
    state: ByteArray
    version: Integer
    taken_at: Timestamp
}

------------------------------------------------------------
-- Contracts
------------------------------------------------------------

contract DeterministicEvaluation {
    evaluate: (event_name: String, payload: ByteArray, current_state: ByteArray) -> EventOutcome

    @invariant Determinism
        -- For identical inputs (event_name, payload, current_state),
        -- evaluate must produce byte-identical EventOutcome values
        -- across all instances and invocations.

    @invariant Purity
        -- evaluate must not perform I/O, read the system clock,
        -- access mutable state outside its arguments, or depend
        -- on the order of previous invocations.

    @invariant TotalFunction
        -- evaluate must return a valid EventOutcome for every
        -- combination of registered event_name, well-formed payload
        -- and current_state. It must not throw or fail to terminate.

    @guidance
        -- Implementations should avoid allocating during evaluation
        -- where possible, as the framework may invoke evaluate
        -- at high frequency during replay.
}

contract EventSubmitter {
    submit: (idempotency_key: String, event_name: String, payload: ByteArray) -> EventSubmission

    @invariant AtMostOnceProcessing
        -- Within the submission TTL window (config.submission_ttl),
        -- a given idempotency key is accepted at most once.
        -- Duplicate submissions are rejected.

    @invariant OrderPreservation
        -- Events submitted by a single module are processed in
        -- submission order. No ordering guarantee exists across
        -- modules.
}

contract StateSnapshots {
    request_snapshot: (entity_key: EntityKey) -> Snapshot
    get_snapshot: (request: SnapshotRequest) -> Snapshot?

    @invariant SnapshotConsistency
        -- A snapshot reflects the state after applying all events
        -- up to and including the snapshot's version number.
        -- No partial application.
}

------------------------------------------------------------
-- Entities
------------------------------------------------------------

entity DomainModule {
    name: String
    version: String
    status: registered | active | suspended

    -- Relationships
    event_types: EventTypeRegistration with module = this

    -- Projections
    active_event_types: event_types where status = active
}

entity EventTypeRegistration {
    module: DomainModule
    event_name: String
    schema_hash: String
    status: active | deprecated
    registered_at: Timestamp
}

entity EventSubmission {
    module: DomainModule
    idempotency_key: String
    event_name: String
    payload: ByteArray
    status: pending | accepted
    submitted_at: Timestamp
    processed_at: Timestamp?

    invariant PayloadWithinLimit { length(payload) <= config.max_payload_bytes }
}

------------------------------------------------------------
-- Config
------------------------------------------------------------

config {
    submission_ttl: Duration = 24.hours
    max_payload_bytes: Integer = 1_000_000
}

------------------------------------------------------------
-- Rules
------------------------------------------------------------

rule RegisterModule {
    when: RegisterModule(module_name, version, event_types)

    requires: not exists DomainModule{name: module_name}

    ensures:
        let module = DomainModule.created(
            name: module_name,
            version: version,
            status: registered
        )
        for event_name in event_types:
            EventTypeRegistration.created(
                module: module,
                event_name: event_name,
                schema_hash: hash(event_name + version),
                status: active,
                registered_at: now
            )
}

rule ActivateModule {
    when: module: DomainModule.status becomes registered

    requires: module.event_types.count > 0

    ensures: module.status = active
}

rule SubmitEvent {
    when: SubmitEvent(module, idempotency_key, event_name, payload)

    let existing = EventSubmission{module: module, idempotency_key: idempotency_key}

    requires: module.status = active
    requires: not exists existing
    requires: exists EventTypeRegistration{module: module, event_name: event_name, status: active}
    requires: length(payload) <= config.max_payload_bytes

    ensures: EventSubmission.created(
        module: module,
        idempotency_key: idempotency_key,
        event_name: event_name,
        payload: payload,
        status: pending,
        submitted_at: now
    )
}

rule ProcessSubmission {
    when: submission: EventSubmission.status becomes pending

    ensures: submission.status = accepted
    ensures: submission.processed_at = now
}

rule ExpireOldSubmissions {
    when: submission: EventSubmission.submitted_at + config.submission_ttl <= now

    requires: submission.status in {pending, accepted}

    ensures: not exists submission
}

rule DeprecateEventType {
    when: DeprecateEventType(module, event_name)

    let registration = EventTypeRegistration{module: module, event_name: event_name}

    requires: exists registration
    requires: registration.status = active

    ensures: registration.status = deprecated
}

rule SuspendModule {
    when: SuspendModule(admin, module, reason)

    requires: module.status = active

    ensures: module.status = suspended
    ensures: AuditLog.created(
        event: module_suspended,
        timestamp: now,
        metadata: { module: module.name, reason: reason, by: admin }
    )
}

rule ReactivateModule {
    when: ReactivateModule(admin, module)

    requires: module.status = suspended
    requires: module.event_types.count > 0

    ensures: module.status = active
}

------------------------------------------------------------
-- Actor Declarations
------------------------------------------------------------

actor FrameworkRuntime {
    identified_by: DomainModule where status = active
}

------------------------------------------------------------
-- Surfaces
------------------------------------------------------------

-- User-facing surface for module administration
surface ModuleAdministration {
    facing admin: User

    exposes:
        for module in DomainModules:
            module.name
            module.version
            module.status
            module.active_event_types.count

    provides:
        RegisterModule(module_name, version, event_types)
        for module in DomainModules where status = active:
            SuspendModule(admin, module, reason)
            for registration in module.active_event_types:
                DeprecateEventType(module, registration.event_name)
        for module in DomainModules where status = suspended:
            ReactivateModule(admin, module)
}

-- Programmatic surface: the framework-to-module integration contract
surface EventSourcingIntegration {
    facing runtime: FrameworkRuntime

    context module: DomainModule where status = active

    contracts:
        demands DeterministicEvaluation
        fulfils EventSubmitter
        fulfils StateSnapshots

    @guarantee ModuleBoundaryIsolation
        -- Events and state from one module are never visible to
        -- another module's evaluate function. Cross-module
        -- communication happens only through side effects processed
        -- by the framework.
}
```

**Key language features shown:**
- `contract` declarations at module level for reuse across surfaces
- Surface `contracts:` clause with `demands`/`fulfils` direction markers (`demands DeterministicEvaluation`, `fulfils EventSubmitter`) without repeating signatures or invariants
- Expression-bearing `invariant Name { expression }` on entities (`PayloadWithinLimit` on `EventSubmission`)
- Prose-only `@invariant Name` inside contracts for properties that cannot be expressed as a single boolean expression
- `@guarantee Name` at surface level, distinct from contract-scoped invariants (boundary-wide vs contract-scoped assertions)
- `@guidance` inside a contract for non-normative implementation advice
- Mixed surface: `ModuleAdministration` uses traditional `exposes`/`provides` for human actors; `EventSourcingIntegration` uses `contracts:` clause for programmatic integration
- Actor declaration for a code-level party (`FrameworkRuntime` identified by an active module)

### When to use contracts

Use `contract` declarations when the boundary is between code and code rather than between a user and an application. All contracts are declared at module level and referenced in surfaces via a `contracts:` clause with `demands`/`fulfils` direction markers. Common scenarios:

- **Framework-to-plugin contracts**: the framework demands evaluation logic, fulfils lifecycle services
- **Service-to-adapter boundaries**: the service demands a storage adapter, fulfils a query interface
- **Cross-context integration**: one bounded context demands event handlers, fulfils event streams
- **SDK contracts**: the SDK demands configuration and callbacks, fulfils client operations

Do not use contracts for user-facing surfaces. If the external party is a person interacting through a UI, use `exposes` (what they see) and `provides` (what actions they can take). Contracts describe what code must implement, not what users can do.

### Contracts vs provides

`provides:` lists actions that an actor can invoke, each corresponding to a rule's external stimulus trigger. `fulfils ContractName` in a `contracts:` clause declares a set of typed operations that the surface owner supplies to the counterpart as an API. The distinction:

- `provides: SubmitEvent(module, key, name, payload)` — an action the actor triggers; a rule fires in response
- `fulfils EventSubmitter` — a typed operation set the surface makes available, defined in a `contract` declaration; the implementation is the surface owner's responsibility

Both describe things the surface supplies, but `provides` connects to the rule system while `fulfils` references a programmatic contract with typed signatures and invariants.

### Invariant vs guarantee

`@guarantee` asserts a property of the surface boundary as a whole. `invariant` asserts a property scoped to the operations within a specific contract.

Invariants come in two forms. Expression-bearing invariants carry a boolean expression that can be checked mechanically. Prose invariants describe properties that require human or LLM judgement.

```
-- Expression-bearing invariant on an entity
entity EventSubmission {
    ...
    invariant PayloadWithinLimit { length(payload) <= config.max_payload_bytes }
}

-- Prose invariant inside a contract
contract DeterministicEvaluation {
    @invariant Purity
        -- evaluate must not perform I/O or access mutable state.
}

-- Surface-level guarantee: applies across the entire boundary
@guarantee ModuleBoundaryIsolation
    -- Events from one module are never visible to another module.
```

Use `@guarantee` for cross-cutting properties that span the whole surface. Use `invariant` for properties tied to specific operations within a contract, or for entity-level assertions.

---

## Using These Patterns

### Composition

Patterns can be composed. For example, a complete document collaboration spec might use:

```
use "./rbac.allium" as rbac
use "./soft-delete.allium" as trash
use "./comments.allium" as comments
use "./notifications.allium" as notify

entity Document {
    workspace: Workspace
    title: String
    content: String
    status: active | deleted
    deleted_at: Timestamp?
    deleted_by: User?

    -- From comments pattern
    comments: comments/Comment with document = this

    -- From soft-delete pattern
    retention_expires_at: deleted_at + trash/config.retention_period
    can_restore: status = deleted and retention_expires_at > now
    ...
}

-- Document actions require RBAC checks
rule EditDocument {
    when: EditDocument(user, document, content)

    let share = rbac/ResourceShare{resource: document, user: user}

    requires: share.can_edit
    ...
}
```

### Adaptation

Patterns are starting points. When applying:

1. **Rename** to match your domain (User → Member, Document → Note)
2. **Adjust** timeouts and limits to your context
3. **Remove** unused states or rules
4. **Extend** with domain-specific behaviour
5. **Compose** multiple patterns for richer functionality

### Anti-Patterns

When using patterns, avoid:

- **Over-engineering**: Don't include reaction system if you don't need reactions
- **Premature abstraction**: Start concrete, extract patterns when you see repetition
- **Pattern worship**: If the pattern doesn't fit, adapt it or write something custom
- **Ignoring context**: A free tier pattern that makes sense for B2C may not fit B2B
