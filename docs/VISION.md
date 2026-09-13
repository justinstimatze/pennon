# Vision

pennon exists to solve one problem: each worktree in a personal fleet
needs its own Linear identity, instead of every one of them posting
under the operator's personal token. That's the actual scope. The rest
of this document is the near-term path for extending it, kept to what's
actually planned.

## Why delegate, not just assignee

An agent handed work by an orchestrator becomes the issue's `delegate`;
the human `assignee` still owns the outcome. That split is already
built and already useful for exactly one reason: it lets a human stay
accountable for an issue without having to be the one who did the
work, and it costs nothing to add — Linear already models both fields.
Self-directed work (an agent claiming its own next item) uses
`assignee` instead, the same primitive a human uses for their own
ticket. No ledger, no reputation model, no cross-org apparatus needed
for either case — this is two fields on an issue, used for what
they're for.

## Where this is genuinely headed

**Already built:** one identity per agent (`mint`/`onboard`), the
watcher and its specialist-review gate, and `gate-poll` for when no
public endpoint exists. Full detail in `docs/ARCHITECTURE.md`.

**Next, concretely:** three things need verifying against Linear's
current API before building further on them — the exact mutation for
external URLs on an Agent Session, the mutation for an app opening a
session on an issue it picks up unassigned (self-directed work, not
delegated), and whether an app-authored mention opens a session on the
mentioned app the same way a human's mention does. None of this is
exotic; it's filling in the parts of the existing mint/onboard/watcher
mechanism that haven't been exercised live yet.

**Real future scope, waiting on a real trigger:** a directory service, if a
question like "how often does this agent get reverted" ever actually
gets asked; promoting the code-review gate from advisory to veto, if
that turns out to matter in practice. Both wait for a real need, not a
plan to build ahead of one.

## What's deliberately not here

Reputation ledgers, escrowed bonds, witness co-signatures across
operators, portable cross-workspace identity — none of that is being
built here, and this document isn't quietly assuming it will be. That
machinery only earns its keep once a genuinely separate
organization's agent needs scope in this workspace. README already
says so plainly: out of scope until pennon is proven inside one
workspace, among people who already trust each other.

Worth designing when a real counterparty exists. Not before.
