# pennon

A pennon is the small flag a knight carried to mark whose colors he fought under — visible identity on a battlefield where you can't otherwise tell who's inside the armor.

That's the problem this project exists to solve: right now, every AI coding agent that touches Linear posts under one human's personal API token. A comment from an agent deep in an overnight run and a comment from the human who owns the workspace are indistinguishable. Two people running independent fleets of agents can't tell, from the ticket history alone, which agent said what, which one is accountable for it, or whether a "sign-off" was a human's judgment or a bot rubber-stamping itself.

pennon gives each agent its own identity in Linear — visible, attributable, and distinct from the humans who operate it — without requiring every agent to be a named specialist, without needing a paid seat per agent, and without two fleets' owners ever having to merge their CLAUDE.md files, memory, or local configuration to work the same ticket.

## The shape of it

- **Each agent is its own OAuth identity.** Agents authenticate via Linear's own `actor=app` mechanism and post as themselves — `venus@justin`, `mars@alex` — distinct from the human who installed them, and without counting as a billable seat.
- **Delegate for orchestrator-initiated work, assignee for self-directed work.** An agent handed an issue becomes the `delegate`; the human `assignee` still owns the outcome. An agent claiming its own next item off a backlog sets `assignee` instead, the same primitive a human uses for their own ticket.
- **Specialists run as a shared service.** A security or QA reviewer doesn't need to exist inside every fleet — one identity, owned by whoever maintains it, gets pulled in across fleets on demand, gated by Linear's workflow states (a ticket enters "Security Review," the specialist is delegated, it signs off or bounces the ticket back). Whether a specialist carries veto power or stays advisory is a decision made per specialist.
- **Identity minting is centralized.** No agent can spawn a new "security reviewer" persona and sign off on its own work. Only the control plane — the orchestrating identity — can create a new specialist identity or grant the scopes that let one sign anything.
- **One small control plane, layered on top of Linear.** Linear stays the system of record. The layer this project adds is a webhook receiver (turns Linear's `AgentSessionEvent`s into local dispatch — nothing else listens for those today) plus a small directory of who's allowed to be mentioned, what they're allowed to do, and who's accountable for them.

## What this isn't

pennon stays a layer on top of Linear, which stays your tracker — it doesn't run your whole company the way an org-chart platform does (see Paperclip).

Its relationship to [ettle](../ettle) is a boundary: ettle's own design is built around a non-negotiable invariant that every coordination loop returns to a human, and a chunk of what pennon needs (agents delegating to agents on subissues, a specialist vetoing a merge on its own signature) sits on the far side of that line by design. pennon *depends on* ettle for the pieces that don't cross it: the bindable-vs-crux router (is this ours to settle, or does a human have to decide) and the tested Linear Document transport (one document per participant, no shared-write clobbering).

A reputation marketplace and a cross-organization trust protocol are real problems (see Davide Crapis's work on portable agent identity at the Ethereum Foundation), and stay out of scope until pennon is proven inside one workspace, between people who already trust each other.

## Status

Identity minting and onboarding (`pennon mint`/`onboard`/`onboard-hook`) are built and used daily by a live fleet. The Linear webhook watcher and its advisory code-review gate are built and verified end to end against real signed webhooks and Linear's real token endpoint; they aren't yet deployed against a live webhook feed.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design reference — the mechanism, the control plane, and the boundaries this project deliberately doesn't cross. [docs/loop-templates.md](docs/loop-templates.md) has two reusable role templates for running a pennon-identified fleet as a recurring loop, generalized from a real overnight run.
