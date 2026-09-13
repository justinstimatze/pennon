# Loop templates

Generalized from a real overnight run of a pennon-identified fleet on
2026-09-11 — both ran for several hours live before being written down
here, not drafted theoretically. Use them with Claude Code's own
`/loop`, or any equivalent scheduler.

These templates are role framing only, layered on top of pennon's own
mechanics rather than restating them. Both assume the reader's fleet already documents its concrete
identity/coordination-rail setup in its own `CLAUDE.md`/`AGENTS.md` —
which agent, which Linear identity, which dispatch rail — the same way
this project's own `docs/ARCHITECTURE.md` "Agent onboarding" section does for
pennon's reference fleet. Point a new agent at that doc; don't
duplicate it here.

## orchestrator-loop

The role that holds visibility across every agent in the fleet,
without gatekeeping who does what. Each cycle:

- Check what's happening across the fleet — dispatch `peek()`, the
  live roster (`who()`), each agent's own recent commits or activity.
- Verify any reported result independently before trusting it — a
  claimed PR, a claimed fix — against the actual artifact (`gh`,
  `git log`), never the report at face value.
- Update the ticket system with verified state, not reported state.
- Surface to the human operator only on completion, a genuine block,
  or fleet-wide idle — not every cycle.
- Stay hands-off on coordination: agents self-assign (claim a real
  ticket via the tracker's own assignee field — durable, fleet-visible
  to everyone, not just you); for work with no ticket shape yet (an ad
  hoc queue, a checklist), an agent announces on dispatch before
  starting instead of you assigning it.
- Your job is unblocking, verifying, and root-causing — never deciding
  who picks up what.

## agent-self-directed

The role for an agent working its own backlog without waiting on
delegation. Each cycle:

- Self-assign the next item — an owned or unassigned ticket, or the
  next entry in an untracked work queue.
- Claim before starting, every time — set assignee, or announce on
  dispatch when there's no ticket to claim. This is the fix for a real
  collision: two agents independently picking the same untracked-queue
  item within minutes of each other, caught only by luck.
- Root-cause before filing. A finding that's actually a tooling
  artifact or a host-pressure false positive isn't a bug.
- Verify any result — including your own prior conclusions — against
  the actual artifact before trusting it.
- Keep the fallback cadence short (minutes, not tens of minutes). An
  agent that only checks in every 20-30 minutes defeats the point of
  self-direction.
- When the queue is genuinely dry, say so and hold. Widen the
  interval — don't manufacture work to look busy.

## What's deliberately left out

No enforcement, no scheduler, no new pennon subcommand — these are
prompt-shaped defaults for whatever already runs your loop.

Copy the role framing, fill in your own fleet's concrete nouns (which
tracker, which rail, which repo), and keep the result in your own
fleet's `CLAUDE.md`/`AGENTS.md` tree rather than reconstructing it
fresh each session. Same principle as everything else pennon defers to
local config: these stay capabilities on offer, decided by whoever
configures them.
