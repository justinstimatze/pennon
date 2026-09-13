# Architecture

pennon gives each AI agent its own identity in Linear, distinct from the
human who operates it. This document is the settled design reference —
the mechanism, the control plane, and the boundaries pennon deliberately
doesn't cross. See [README.md](../README.md) for the pitch and current
status, and [docs/loop-templates.md](loop-templates.md) for reusable
loop-role prompts once a fleet is identified this way.

## Core mechanism

Verified directly against Linear's own OAuth and agent docs, not a
secondhand summary:

- **`actor=app` OAuth authorization.** Adding `actor=app` to the OAuth
  authorization URL makes the resulting token post as the installed
  application itself, not the installing human. Agents don't count as
  billable seats. This is set once, at authorization — every later
  `client_credentials` mint for that same Application inherits the
  property automatically, with no `actor=` parameter needed at mint
  time (see "Agent onboarding" for the mint path pennon actually runs).
- **Two tiers of identity.** *Lightweight*: one app install, every
  mutation carries `createAsUser`/`displayIconUrl` — renders as
  `AgentName (via YourApp)`. Cosmetic only: not mentionable, not
  assignable, and anyone holding the app token can claim any name.
  *Full*: a separate OAuth Application per named identity, with
  `app:mentionable`/`app:assignable` scopes — a real actor, mentionable
  and delegatable, its name and icon are how it appears in Linear's own
  UI.
- **Delegate for orchestrator-initiated work, assignee for self-directed
  work.** Assigning an issue to an app sets `delegate`: the human keeps
  nominal ownership and liability, the agent does the work. This is the
  right field for human- or orchestrator-initiated work. For an agent
  claiming its own next item off a backlog, `assignee` is the correct
  primitive instead — the same claim mechanism a human uses for their
  own ticket, confirmed by real self-directed fleet use. Linear enforces
  each field's own access-control semantics once it's set; pennon
  documents which field to use for which case, but no code in this
  repo sets, reads, or checks either field today.
- **Agent Sessions.** Once an agent is mentioned or delegated, Linear
  auto-creates an Agent Session tracking that task's lifecycle from the
  agent's own emitted activity, visible to everyone with no manual
  state sync. `awaitingInput` is a real human-to-agent (or
  agent-to-agent) escalation channel. Sessions can also be created
  proactively (`agentSessionCreateOnIssue`/`OnComment`) without a prior
  mention or delegation.
- **Labels, projects, and custom fields carry no enforcement.** They're
  data. The one enforcement point pennon itself builds is the
  watcher, which can refuse to route a delegation and say so. GitHub
  branch protection is a real second enforcement point an adopter can
  use the same way, but it's their own external setup — pennon has no
  code that configures or verifies it. A label is metadata the watcher
  reads, never itself the gate.

## Identity granularity

One Linear identity per persistent agent, `<agent>@<human>` (`venus@justin`,
`mars@alex`) — the identity binds to the agent's worktree, not to
whether a session happens to be running. An orchestrating identity
(a "mayor" role that does planning and re-delegates internally) gets
its own name too, not folded into the agents it coordinates.

Ephemeral worktrees and specialist gates need a different shape than a
persistent agent: a small fixed pool of placeholder identities, leased
with an expiry rather than trusted to be released explicitly (an
abandoned worktree shouldn't leak its slot forever). Specialists pool
by *type*, not by one persistent identity per specialist — narrow,
stateless, checked out per gate-check — with one exception (see "Code
review specialist" below).

**A real constraint on all of this:** `client_credentials` can mint a
token with `app:mentionable`/`app:assignable` already granted, no
browser step required — this half is fully automatable (see
"Automating per-agent app creation"). Creating the OAuth Application
object itself per identity is not currently self-service; see that
section for the exact gate.

## Identity minting is centralized

An implementer must not be able to spawn its own reviewer persona and
sign off on its own work. Real enforcement backs that now: every
`Specialist` identity carries a `Kind` field, and `pennon mint`/`onboard`
refuse to touch any identity marked `Kind: "specialist"` — those are
provisioned only through the gate/watcher's own Delegate flow (see "The
watcher"). Fixed 2026-09-13, found live: a specialist identity sitting
unmarked in the same flat `identities.json` as ordinary fleet agents let
any of them self-mint the specialist's own token, exactly the bypass
this invariant exists to prevent — closing that gap is what turned this
into a real enforcement point.

Whether a specialist carries veto or advisory power is a real
per-specialist decision, made explicitly, never a default. Revoking a
specialist identity must invalidate standing authority going forward —
anything it already signed gets checked against *current* standing at
merge time, not treated as permanently valid because it was valid when
signed. That revocation check isn't built yet; today, an already-minted
specialist token stays valid until Linear itself expires it.

## The control plane

The pieces Linear itself doesn't provide:

- **The watcher.** An always-on receiver for Linear's
  `AgentSessionEvent`s and `Issue` webhooks — Claude Code sessions
  don't listen for webhooks themselves. See "The watcher" below for
  its own deadlines and shape.
- **The directory (not yet built).** A small store — identity, owning
  operator, current scope, who's allowed to mint a new one. Also
  answers "who do I hand this off to" once that stops being obvious
  from context alone.
- Enforcement pennon builds lives in the watcher's routing decisions,
  never in metadata alone; GitHub branch protection is a real second
  option but it's the adopter's own external setup, not something this
  repo configures (see "Core mechanism" above).

## The watcher

`cmd/pennon` runs the watcher as a real HTTP server
(`PENNON_WEBHOOK_SECRET`, `PENNON_LISTEN_ADDR`), built and verified end
to end against real signed webhooks — not yet deployed against a live
Linear webhook feed.

Two real deadlines govern it, both confirmed against Linear's own
docs: the receiver must acknowledge a webhook within 5 seconds, and on
a `created` `AgentSessionEvent`, the agent must emit an activity or set
an external URL within 10 seconds or Linear marks the session
unresponsive. These are two different clocks — the watcher's own ack,
and the agent's first response — and both are real.

Delivery is at-least-once and unordered, so every event needs an
idempotency key; `internal/watcher` dedupes by Linear's own delivery
id. A restart currently loses the in-memory dedupe set — acceptable
only because nothing is deployed against a live webhook yet, worth
revisiting before that changes.

`internal/watcher` also decodes Linear's `Issue` webhooks and gates on
them: `GateRouter` matches a (team, workflow-state) pair against a
`GateSet`, confirms the update is a genuine transition into that state
(not just any edit while already sitting there), and delegates to the
matched specialist via a real `client_credentials` mint followed by
`agentSessionCreateOnIssue`.

## Polling instead of the watcher

The watcher needs something reachable from the public internet — a
tunnel (Cloudflare Tunnel, ngrok) in front of a box behind NAT, or a
real public IP — because Linear's webhook delivery is Linear's servers
pushing to a URL you register; nothing about that is optional. When
that's more infrastructure than a deployment wants yet, `pennon
gate-poll` gets the same outcome from the other direction: it reads the
same `GateSet` the watcher's `GateRouter` does, asks Linear which issues
currently sit in each gate's (team, state) pair, and delegates any that
aren't already delegated to that gate's specialist. No inbound
connection, ever — it only makes outbound calls, on whatever schedule
something else drives it (cron, a systemd timer, a Claude Code `/loop`).
`gate-poll` runs one cycle and exits; it is not a second daemon
alongside the watcher.

The tradeoff shows up as latency: a ticket sits in the gated state
until the next poll fires, rather than being delegated within seconds
of the transition. Checking the issue's current `delegate` before
acting (rather than diffing against a previous poll's state, which
polling has no memory of) is what keeps repeated cycles from
re-delegating the same issue — an issue already delegated to the gate's
specialist is left alone every time it's seen again.

Needs its own token via `PENNON_POLL_TOKEN`, deliberately separate from
any specialist's own mint: listing every issue in a team's state is a
read across the whole team, a different kind of act than anything done
as that specialist.

## Deployment config lives outside pennon's own repo

No team id, workflow-state id, or specialist identity name is a
literal anywhere in pennon's source. The gate mapping is runtime
config, loaded from a file path given by `PENNON_GATE_CONFIG` (see
`config/gate.example.json` for the shape); real values — a real team
id, a real specialist's credentials — live locally on whatever host
runs the deployment, gitignored, never committed. This is deliberate:
pennon stays adoptable by anyone without leaking any one deployment's
specifics into the general design, and an unset `PENNON_GATE_CONFIG`
just means Issue webhooks are acked with no gate checking attempted,
never an error.

A private config repo with secrets-manager support is a real upgrade
path once config needs to be shared or reviewed across more than one
deployment — not built ahead of an actual second adopter needing it.

## Agent onboarding

`pennon onboard <name>` mints a token, verifies it against a live
`viewer` query before touching anything, then patches both `.mcp.json`
and `.claude/settings.local.json` in place (atomic writes, timestamped
backups). It patches an existing `.mcp.json` — it doesn't scaffold one —
so `.mcp.json` needs `linear` and `linear-notifications` server entries
already present before onboarding a worktree; either is missing, onboard
warns and skips that entry rather than inventing it. `linear-notifications`
is also what makes an identity's own notification inbox
(`getNotifications`, `getUnreadNotificationCount`) queryable from inside
its session with its own token — the same channel a persistent lane can
poll for its own @mentions and delegations without any watcher running
at all. What onboard still can't automate: reconnecting the MCP server
in Claude Code, and checking Linear team membership (a freshly minted
identity starts on no team, which 403s on team-scoped calls) — both
print as the command's last lines rather than being silently skipped.

`pennon onboard-hook <name>` wires this to Claude Code's `SessionStart`
hook so an already-onboarded worktree refreshes its identity's token
automatically: debounces (12h default, keyed per identity — a
refresh is idempotent, so this is deliberately unlike a
per-session key), and when due, spawns `onboard` detached so a
session's own startup never blocks on the mint/verify network
round-trip. Confirmed live: re-minting a token doesn't invalidate one
a live session's MCP server already holds, so a background refresh
can't disconnect an already-connected session.

Point a freshly onboarded identity at its host repo's own
`CLAUDE.md`/`AGENTS.md`: every worktree of a repo inherits that file
automatically on session start, with no watcher or API call involved —
the cheapest distribution mechanism this project depends on for any
standing operational convention. `onboard` prints a reminder to use it
for exactly this reason.

`onboard` records the identity it mints in
`.claude/settings.local.json`'s `env.PENNON_IDENTITY`, and `mint`/`onboard`
refuse to mint any other name once that's set — a worktree can only ever
mint the identity it already declared for itself. Two things this
doesn't do: it doesn't defend against a fully compromised process
willing to delete or edit its own `.claude/settings.local.json` to reset
back to the undeclared state, the same way the specialist guard doesn't
defend against tampering with the shared `identities.json` directly (see
"Trust model"); and it isn't automatic for a worktree onboarded before
this existed — that worktree has `ETTLE_ME` set but not
`PENNON_IDENTITY`, and gets no protection from this guard until its next
`onboard` run declares it.

## Dependency on mcp-dispatch

[mcp-dispatch](https://github.com/justinstimatze/mcp-dispatch) is the
agent-to-agent messaging rail this project runs on and depends on
directly rather than reimplementing a smaller version of the same
thing. The watcher's Linear-to-agent bridge routes onto it via
`bin/dispatch-send` (`internal/watcher.DispatchRouter`) rather than a
bespoke queue — verified once, manually, end to end against a real
signed webhook and the real `dispatch-send` binary. The automated test
suite doesn't re-exercise that binary on every run; it stubs
`dispatch-send` with a fake script that just records its arguments.
`cmd/pennon` picks `DispatchRouter` over
a log-only fallback automatically when `PENNON_DISPATCH_SEND` is set;
unset, events are only logged and acked. This binary shouldn't
require the sibling project just to run locally. There's no per-agent
directory yet, so every routed event currently goes to one fixed
destination (`PENNON_DISPATCH_TO`).

## Dependency on ettle

[ettle](https://github.com/justinstimatze/ettle) is a sibling project.
Its own design invariant ("the agent mesh is instrumentation for human
coordination, never a substitute for it") rules out exactly the things
pennon needs (agents delegating to
agents without a mandatory human return, a specialist vetoing a merge
on its own signature). What pennon reuses instead of reimplementing:
the bindable-vs-crux distinction (is this ours to settle, or does a
human decide) applied to delegate-vs-assignee and advisory-vs-veto;
and the Linear Document transport pattern (one document per
participant, sidestepping the write-clobber problem a shared document
would have) via a CLI subprocess boundary
(`ettle linear-doc upsert`) rather than a Go import — `internal/`
packages don't cross module boundaries in Go, and a subprocess
boundary can't leak ettle's other scope by accident the way an import
could.

## Automating per-agent app creation

Two separate questions, tested live against a real (non-production)
Linear workspace:

- **Granting an agent its scopes is fully automatable.** A plain
  `client_credentials` request against an existing OAuth Application,
  scoped to `app:mentionable app:assignable`, returns both scopes
  granted — no consent screen, no human click.
- **Creating the OAuth Application object itself is not self-service.**
  Linear's schema marks `oauthApplicationCreate` `[ALPHA]` — a
  parent-app-creates-child-apps mechanism gated behind an `oauth:create`
  scope that isn't in any documented grant path. Each identity's OAuth
  Application still needs one manual registration; everything
  downstream of that (minting, scope-granting) is already proven
  scriptable. At small fleet sizes this is a one-time cost per identity.

## Code review specialist

Gate policy starts advisory (the specialist posts a review, nothing
blocks on it) with an explicit intent to promote to veto once the gate
sees real use — the policy is carried on `Gate.Policy` from day one so
the promotion is just a config change. Identity model is a
small named series rather than one pooled identity behind a shared
name — deliberately different from the specialist-pool-by-type default
above, chosen so review activity in Linear visibly comes from more
than one interchangeable bot and each can be iterated on independently.

## Fabro

[Fabro](https://github.com/fabro-sh/fabro) is a real, MIT-licensed
workflow-graph runner that's a plausible execution engine for a code
review specialist's actual review logic — but running it means
standing up a server, which fails pennon's own adoption bar (Linear +
GitHub access only, nothing else to set up). The default code review
specialist is therefore the plain case: a Claude Code (or any harness)
session authenticated as the specialist identity, reading its own
delegated Agent Sessions and posting its own verdict. Fabro stays a
real option for whoever wants heavier workflow-graph machinery later,
not a default dependency — same treatment as `gemot` below.

## Trust model

Built for people who already trust each other — co-workers on the same
paid Linear workspace — not strangers. Installing an identity under
someone's account *is* the vouching act; no reputation ledger, escrow,
or witness co-signature needed for that case. That machinery becomes
relevant the day a genuinely outside organization's agent wants scope
in the workspace — real future scope, not phase-one work.

The mint-time specialist guard (see "Identity minting is centralized")
closes one specific gap — a fleet agent self-minting another identity's
token from a shared local file — but it isn't, and doesn't try to be, a
second layer of access control on top of Linear's own. Who can create or administer an OAuth Application in the workspace at
all is entirely Linear's own permission model. That's the actual root
of trust the whole control plane rests on: pennon assumes whoever runs
it already holds the Linear access needed to create identities in the
first place.

Checked directly and worth stating plainly: neither of Linear's two
OAuth-application mutations (`archiveManagedOAuthApplication`,
`rotateManagedOAuthApplicationSecret`) apply to how pennon provisions
identities today — both are scoped to "alpha child" apps created
programmatically under a managing parent app, not the standalone,
manually-created workspace OAuth Applications pennon's identities
actually are. There is no API pennon could call to immediately
invalidate an already-issued specialist token. A specialist's prior
Agent Session activity also isn't something pennon ever reads back
(see `Policy`'s doc comment in `internal/watcher/gate.go`), so if a
specialist identity is later revoked or demoted, its past sign-offs
stand exactly as posted until Linear's own token expiry (documented as
up to 30 days) — re-reviewing that history is a manual call for
whoever consumes the verdict.

## Adoption principle

pennon should never require an adopter to take on a new third-party-
hosted dependency — no new SaaS vendor, no new paid service, no new
external database to run. `go.mod` has zero external dependencies,
which is the real thing this principle is checked against. It doesn't
mean adopting pennon is free of new moving parts: minting a fresh OAuth
client secret per identity is pennon's whole purpose, and running the
watcher means self-hosting an always-on HTTP server yourself — both are
real, just not *third-party* dependencies. Every heavier third-party
dependency this project could take on gets checked against the bar
above before it's added: it's why the OAuth mechanism was the right
starting point, why a private GitHub repo (via mcp-dispatch's
git-transport) was the right choice for a cross-fleet channel instead
of a hosted message queue, and why heavier optional infrastructure —
[gemot](https://github.com/justinstimatze/gemot)'s LLM-mediated
multi-party deliberation, Fabro's workflow-graph runner — stays roadmap
rather than core: both need their own server and their own database,
which is real adoption friction.

## What's built vs. what's ahead

Built and used daily:

- `pennon mint`/`onboard`/`onboard-hook`, refusing to mint any `Kind: "specialist"` identity or any identity other than the one a worktree already declared for itself
- The watcher and gate, verified end to end — not yet deployed against a real webhook feed
- `pennon gate-poll`, the deploy-free stand-in for the watcher's gate — no public endpoint needed, run on any schedule
- Deployment config held outside the repo

Still ahead:

- The directory — triggered by the first real "how often does this agent get reverted" question
- Per-agent OAuth Application registration, manual until Linear grants broader `oauth:create` access
- Promoting the code-review gate from advisory to veto
- The full stranger-trust apparatus, deliberately not built until an actual outside party needs it
