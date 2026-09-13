package watcher

// Policy is what's expected to happen with a gate's verdict once the
// specialist posts it. Pennon's own job stops at delegating — enforcing
// veto (blocking further progress on the issue) is the specialist's own
// harness's job, or whatever process consumes its verdict; Policy is
// carried through so that consumer knows which behavior is expected of
// it, not so pennon can enforce it itself.
//
// A real consequence of that boundary: pennon never reads a specialist's
// past verdict back, so it has nothing to re-check if that identity is
// later revoked or demoted. If a specialist's credentials are rotated or
// its Kind is unmarked, its prior Agent Session activity on already-
// reviewed issues stays exactly as posted — Linear has no API to
// retroactively invalidate that history, and pennon's design keeps
// verdict-consumption outside its own job on purpose (see docs/
// ARCHITECTURE.md "Trust model"). Re-reviewing an issue a now-untrusted
// specialist already signed off on is a manual call for whoever consumes
// the verdict, not something a later pennon run can detect for you.
type Policy string

const (
	PolicyAdvisory Policy = "advisory"
	PolicyVeto     Policy = "veto"
)

// Specialist is the credentials a gate delegates to — one OAuth
// Application per named specialist identity, matching the identity
// model settled in docs/ARCHITECTURE.md ("Code review specialist" — a small named
// series, not one pooled identity behind a shared name).
type Specialist struct {
	Name         string `json:"name"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`

	// Kind marks an identity's provisioning boundary. Empty (equivalent
	// to "agent") identities are ordinary fleet identities, mintable by
	// anyone with mint/onboard access to the identities file they're
	// listed in. "specialist" identities are provisioned only through
	// the gate/watcher's own Delegate flow (see Gate.Specialist) —
	// mint/onboard refuses to touch them. This is the fix for a real gap
	// found live 2026-09-13: a specialist identity sitting in the same
	// flat identities.json as ordinary fleet agents let any of them
	// self-mint the specialist's own token — exactly the self-review
	// bypass "identity minting is centralized" is supposed to prevent.
	// See docs/ARCHITECTURE.md "Identity minting is centralized."
	Kind string `json:"kind,omitempty"`
}

// KindSpecialist is the Specialist.Kind value that mint/onboard refuses to
// touch.
const KindSpecialist = "specialist"

// IsSpecialist reports whether this identity is provisioned only through
// the gate/watcher's own Delegate flow, not mint/onboard.
func (s Specialist) IsSpecialist() bool { return s.Kind == KindSpecialist }

// Gate maps one team's workflow state to the specialist that should
// review an issue arriving there, and the policy that verdict carries.
// There is no hardcoded team, state, or specialist name anywhere in
// pennon's source — see docs/ARCHITECTURE.md "Deployment config lives outside
// pennon's own repo": every value here comes from a config file loaded
// at runtime by LoadGates.
type Gate struct {
	TeamID     string     `json:"team_id"`
	StateID    string     `json:"state_id"`
	Policy     Policy     `json:"policy"`
	Specialist Specialist `json:"specialist"`
}

// GateSet is the full set of configured gates for one deployment.
type GateSet []Gate

// Match returns the first gate configured for (teamID, stateID), if any.
func (g GateSet) Match(teamID, stateID string) (Gate, bool) {
	for _, gate := range g {
		if gate.TeamID == teamID && gate.StateID == stateID {
			return gate, true
		}
	}
	return Gate{}, false
}
