# Contributing

pennon is a small project maintained by [@justinstimatze](https://github.com/justinstimatze). PRs and issues welcome.

## Ground rules

These come from the design in [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — read it before touching the control plane.

- **Identity minting is centralized.** `mint`/`onboard` refuse to touch any identity marked `kind: specialist` — those are provisioned only through the gate/watcher's own Delegate flow, closing off the direct path an implementer would otherwise have to spawn and mint its own reviewer persona.
- **Delegate for orchestrator-initiated work, assignee for self-directed work.** An agent taking an issue handed to it sets `delegate` — the human keeps nominal ownership. An agent claiming its own next item off a backlog sets `assignee` instead, the same primitive a human uses for their own ticket. Linear enforces each field's own access-control semantics once it's set; pennon's own code doesn't set or check either field today, so don't assume it will catch a mix-up — keep the distinction in mind if you add code that does.
- **Enforcement, where it exists, lives in the watcher's routing decisions** — it can refuse to route a delegation and say so as a visible activity. GitHub branch protection is a real enforcement point too, but it's the adopter's own external setup; pennon has no code that configures or verifies it. A label or custom field is metadata to read, never itself a gate.
- **No agent-to-agent mention loop without a hop-count guard.** Anything that can trigger another agent session needs a bounded loop and a human-escalation path on repeated unanswered elicitation.

## Dev setup

```sh
git clone https://github.com/justinstimatze/pennon
cd pennon
go test ./...
go build ./cmd/pennon
```

Runs on the Go version pinned in `go.mod`. CI uses `go-version-file: go.mod`; local dev should track the same. For full lint parity with CI, install `golangci-lint`.

## Tests

Before submitting a PR:

```sh
go test ./...
go vet ./...
golangci-lint run
go build ./cmd/pennon
```

## Commit style

Short imperative subject lines. Body explains why, not what. Reference issues as `#123`.

## Release process (maintainer)

1. Update `CHANGELOG.md`.
2. Tag `vX.Y.Z` on `main`.
3. GitHub release with binaries for linux/darwin × amd64/arm64 (goreleaser, triggered by the tag push).
