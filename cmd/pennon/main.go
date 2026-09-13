// Command pennon is the control-plane entry point described in
// docs/ARCHITECTURE.md. Today it runs the watcher: an HTTP server that receives
// Linear's AgentSessionEvent webhooks, verifies and dedupes them, and
// routes each one — currently to a log line, since the real Dispatch
// route (see internal/watcher.LogRouter) isn't wired up yet.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/justinstimatze/pennon/internal/onboard"
	"github.com/justinstimatze/pennon/internal/watcher"
)

// version is "dev" by default and baked at release time via
//
//	go install -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/pennon
//
// The git tag is the single source of truth — there is no hand-maintained
// version constant to drift out of sync. buildVersion() resolves it.
var version = "dev"

// buildVersion reports the binary's version, preferring (in order): a
// release value baked in via -ldflags; the module version when installed
// with `go install …@vX.Y.Z`; the embedded VCS commit (+dirty) for local
// `go build`. Falls back to "dev" when none is available (e.g. a tarball
// build outside a git tree).
func buildVersion() string {
	if version != "dev" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 12 {
				rev = s.Value[:12]
			} else {
				rev = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev != "" {
		return rev + dirty
	}
	return version
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-version" || os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(buildVersion())
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "mint" {
		if err := runMint(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "pennon:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "onboard" {
		if err := runOnboard(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "pennon:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "onboard-hook" {
		if err := runOnboardHook(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "pennon:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "gate-poll" {
		if err := runGatePoll(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "pennon:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pennon:", err)
		os.Exit(1)
	}
}

// runMint prints a bare, freshly minted access token for one named
// identity to stdout — nothing else, so it's safe to capture directly:
//
//	export LINEAR_API_TOKEN=$(pennon mint mercury@justin)
//
// This is the onboarding path for a worktree's own Claude Code session:
// point its Linear MCP server at that token instead of the shared
// personal access token everyone currently authenticates through (see
// docs/ARCHITECTURE.md's "Agent onboarding" note). The token is an app-actor
// token — Linear itself decides how long it's valid (documented as up
// to 30 days) — so this is meant to run once per session start, not
// once per request.
func runMint(args []string) error {
	if len(args) != 1 || args[0] == "" {
		return errors.New("usage: pennon mint <identity-name>")
	}
	token, err := mintForIdentity(context.Background(), args[0])
	if err != nil {
		return err
	}
	fmt.Println(token)
	return nil
}

// mintForIdentity looks up name in the configured identities file and
// mints it a fresh token — the lookup-and-mint logic shared by both
// `pennon mint` (bare token to stdout) and `pennon onboard` (mint, then
// verify, then wire it into this worktree's own config).
func mintForIdentity(ctx context.Context, name string) (string, error) {
	path := os.Getenv("PENNON_IDENTITIES")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("PENNON_IDENTITIES not set and couldn't resolve a home directory to default from: %w", err)
		}
		path = filepath.Join(home, ".config", "pennon", "identities.json")
	}

	identities, err := watcher.LoadIdentities(path)
	if err != nil {
		return "", err
	}
	identity, ok := watcher.FindIdentity(identities, name)
	if !ok {
		return "", fmt.Errorf("no identity named %q in %s", name, path)
	}
	if identity.IsSpecialist() {
		return "", fmt.Errorf("%q is a specialist identity — specialists are provisioned only through the watcher's own gate/delegate flow, not mint/onboard (see docs/ARCHITECTURE.md \"Identity minting is centralized\")", name)
	}

	settingsPath := filepath.Join(".claude", "settings.local.json")
	declared, err := onboard.DeclaredIdentity(settingsPath)
	if err != nil {
		return "", fmt.Errorf("check declared identity in %s: %w", settingsPath, err)
	}
	if declared != "" && declared != name {
		return "", fmt.Errorf("this worktree is onboarded as %q, not %q — mint/onboard only ever mints the identity a worktree already declared for itself", declared, name)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// A minted identity is meant to fully replace the shared PAT it's
	// onboarding away from — the operator's own read/write access under
	// its own name, not the narrower app:mentionable/app:assignable pair
	// Delegate uses for the gate's specialist-review flow. Confirmed
	// live 2026-09-11 (venus@justin): Linear grants all four scopes
	// together with no separate consent step.
	d := watcher.LinearDelegator{Log: log, Scope: "app:mentionable,app:assignable,read,write"}
	token, err := d.MintToken(ctx, identity)
	if err != nil {
		return "", fmt.Errorf("mint token for %s: %w", name, err)
	}
	return token, nil
}

// runOnboard collapses the manual sequence in docs/ARCHITECTURE.md's "Agent
// onboarding" — mint, verify, wire the token into this worktree's own
// .mcp.json, set ettle's per-session identity — into one command, run
// from inside the worktree it's onboarding. It never prints the token
// itself: this fleet's Claude Code transcripts get mined by other
// tooling (costean, winze), so `mint`'s bare-stdout contract is fine for
// `export TOKEN=$(...)` but wrong for a command whose whole point is
// that the token never needs to leave the process.
//
// Verification gates every file write: a token that fails the live
// viewer check is exactly the bug pennon mint's own scope shipped with
// once already (see docs/ARCHITECTURE.md), so nothing gets wired to a token this
// command hasn't confirmed actually works. Reconnecting the MCP servers
// and Linear team membership stay manual — printed as the last step,
// not attempted here.
func runOnboard(args []string) error {
	if len(args) != 1 || args[0] == "" {
		return errors.New("usage: pennon onboard <identity-name>")
	}
	name := args[0]
	ctx := context.Background()

	token, err := mintForIdentity(ctx, name)
	if err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	viewer, err := (watcher.LinearDelegator{Log: log}).VerifyToken(ctx, token)
	if err != nil {
		return fmt.Errorf("minted a token for %s but it failed a live viewer check, so nothing was wired to it: %w", name, err)
	}
	fmt.Printf("minted and verified %s (linear user %s, id %s)\n", name, viewer.Name, viewer.ID)

	mcpResult, mcpErr := onboard.PatchMCPConfig(".mcp.json", token)
	for _, w := range mcpResult.Warnings {
		fmt.Println("  .mcp.json:", w)
	}
	if mcpErr == nil {
		if mcpResult.Changed {
			fmt.Println("  .mcp.json: updated")
		} else {
			fmt.Println("  .mcp.json: already up to date")
		}
	}

	settingsPath := filepath.Join(".claude", "settings.local.json")
	settingsResult, settingsErr := onboard.PatchSettingsLocal(settingsPath, name)
	for _, w := range settingsResult.Warnings {
		fmt.Println("  "+settingsPath+":", w)
	}
	if settingsErr == nil {
		if settingsResult.Changed {
			fmt.Println("  " + settingsPath + ": updated")
		} else {
			fmt.Println("  " + settingsPath + ": already up to date")
		}
	}

	switch {
	case mcpErr != nil && settingsErr != nil:
		return fmt.Errorf(".mcp.json failed (%w) and %s failed (%w) — rerun `pennon onboard %s`, both patches are idempotent", mcpErr, settingsPath, settingsErr, name)
	case mcpErr != nil:
		return fmt.Errorf("%s was updated but .mcp.json failed: %w — rerun `pennon onboard %s`, both patches are idempotent", settingsPath, mcpErr, name)
	case settingsErr != nil:
		return fmt.Errorf(".mcp.json was updated but %s failed: %w — rerun `pennon onboard %s`, both patches are idempotent", settingsPath, settingsErr, name)
	}

	fmt.Println("\nstill manual:")
	fmt.Println("  - reconnect: /mcp, pick linear and linear-notifications, Reconnect")
	fmt.Println("  - team membership: check get_user \"me\" — if teams is empty, ask your Linear workspace admin to add you to one")
	fmt.Println("  - shared record: this repo's CLAUDE.md/AGENTS.md, if it has one, is what every future session here inherits automatically — write standing operational conventions there instead of relaying them by hand each time")
	return nil
}

// runOnboardHook is `pennon onboard-hook <name>` — the trigger a Claude
// Code SessionStart hook fires so a worktree's identity stays fresh
// without anyone running `pennon onboard` by hand (see docs/ARCHITECTURE.md
// "Agent onboarding"). Debounces (SessionStart refires on resume,
// /clear, and /compact, not just a fresh launch), then spawns `pennon
// onboard <name>` DETACHED and returns immediately — a hook must never
// block the session on a network round-trip, same reasoning as ettle's
// own capture-hook. Re-minting doesn't invalidate a token a live
// session's MCP server already holds — confirmed live 2026-09-10 (two
// mints for the same identity back to back, the first token still
// answered a real query after the second) — so a background refresh
// can't 401 an already-connected session out from under it.
func runOnboardHook(args []string) error {
	fs := flag.NewFlagSet("onboard-hook", flag.ContinueOnError)
	debounce := fs.Duration("debounce", 12*time.Hour, "skip the refresh if onboard-hook already ran for this identity within this window")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || fs.Arg(0) == "" {
		return errors.New("usage: pennon onboard-hook <identity-name>")
	}
	name := fs.Arg(0)

	// SessionStart's hook payload carries fields this command doesn't
	// need (identity comes from the positional arg), but the pipe must
	// still be drained so the caller never blocks on it.
	_, _ = io.Copy(io.Discard, os.Stdin)

	due, err := onboard.Due(name, *debounce)
	if err != nil {
		return err
	}
	if !due {
		return nil
	}

	fmt.Printf("pennon: refreshing %s's Linear identity in the background\n", name)

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "onboard", name)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own process group: survives the hook exiting

	if logFile, err := openOnboardHookLog(); err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		defer func() { _ = logFile.Close() }()
	}
	// A log we can't open isn't a reason to skip the refresh — it just
	// means this run's outcome won't be checkable afterward.

	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// openOnboardHookLog is the only place a detached onboard-hook run's
// outcome becomes checkable — its parent (this hook process) returns
// long before the child finishes, so the child's own stdout/stderr has
// nowhere else to go. Append-only: this file accumulates across every
// refresh, not pruned here.
func openOnboardHookLog() (*os.File, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "pennon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "onboard-hook.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

// run starts the watcher's webhook server and blocks until SIGINT/SIGTERM,
// then shuts down gracefully so an in-flight webhook still gets its
// response inside Linear's 5-second ack window (see docs/ARCHITECTURE.md "The
// watcher") before the process exits.
func run() error {
	secret := os.Getenv("PENNON_WEBHOOK_SECRET")
	if secret == "" {
		return errors.New("PENNON_WEBHOOK_SECRET is required (Linear's webhook signing secret)")
	}
	addr := os.Getenv("PENNON_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	router, err := buildRouter(log)
	if err != nil {
		return err
	}
	issueRouter, err := buildIssueRouter(log)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/webhooks/linear", watcher.Handler(
		[]byte(secret),
		watcher.NewMemoryDeduper(),
		router,
		issueRouter,
		log,
	))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		log.Info("watcher listening", "addr", addr, "version", buildVersion())
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	log.Info("shutting down")
	return srv.Shutdown(shutdownCtx)
}

// buildRouter returns a watcher.DispatchRouter when PENNON_DISPATCH_SEND
// points at mcp-dispatch's bin/dispatch-send (see docs/ARCHITECTURE.md "Dependency
// on mcp-dispatch"), or a watcher.LogRouter otherwise. LogRouter is a
// deliberate fallback, not an error: dispatch-send is a sibling project's
// tool, and requiring it would mean this binary can't even run for local
// testing without another project installed and configured first.
func buildRouter(log *slog.Logger) (watcher.Router, error) {
	sendPath := os.Getenv("PENNON_DISPATCH_SEND")
	if sendPath == "" {
		log.Warn("PENNON_DISPATCH_SEND not set, falling back to LogRouter — events will be logged, not routed anywhere")
		return watcher.LogRouter{Log: log}, nil
	}
	to := os.Getenv("PENNON_DISPATCH_TO")
	if to == "" {
		return nil, errors.New("PENNON_DISPATCH_TO is required when PENNON_DISPATCH_SEND is set (no per-agent directory exists yet, so every event needs one fixed destination — see docs/ARCHITECTURE.md \"The control plane\")")
	}
	from := os.Getenv("PENNON_DISPATCH_FROM")
	if from == "" {
		from = "pennon-watcher"
	}
	return watcher.DispatchRouter{
		SendPath: sendPath,
		From:     from,
		To:       to,
		Log:      log,
	}, nil
}

// buildIssueRouter returns a watcher.GateRouter loaded from
// PENNON_GATE_CONFIG when set, or nil otherwise. Nil is a deliberate,
// documented default (see docs/ARCHITECTURE.md "Deployment config lives outside
// pennon's own repo") — a deployment with no gates configured simply
// acks Issue webhooks without acting on them; pennon's source carries no
// team, state, or specialist identity of its own to fall back to.
func buildIssueRouter(log *slog.Logger) (watcher.IssueRouter, error) {
	path := os.Getenv("PENNON_GATE_CONFIG")
	if path == "" {
		log.Info("PENNON_GATE_CONFIG not set, Issue webhooks will be acked without gate checking")
		return nil, nil
	}
	gates, err := watcher.LoadGates(path)
	if err != nil {
		return nil, err
	}
	return watcher.GateRouter{
		Gates:    gates,
		Delegate: watcher.LinearDelegator{Log: log},
		Log:      log,
	}, nil
}

// runGatePoll runs one gate-poll cycle and exits — the deploy-free
// alternative to the watcher's webhook path, for when standing up
// anything reachable from the public internet is more than the moment
// calls for (see docs/ARCHITECTURE.md "Polling instead of the watcher").
// It's meant to be invoked repeatedly by something else — cron, a
// systemd timer, a Claude Code /loop — not to loop internally itself;
// pennon stays a plain CLI tool either way, not a second daemon to run
// alongside the watcher.
func runGatePoll(args []string) error {
	fs := flag.NewFlagSet("gate-poll", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	gateConfig := os.Getenv("PENNON_GATE_CONFIG")
	if gateConfig == "" {
		return errors.New("PENNON_GATE_CONFIG is required (gate-poll has nothing to check without it)")
	}
	token := os.Getenv("PENNON_POLL_TOKEN")
	if token == "" {
		return errors.New("PENNON_POLL_TOKEN is required (a Linear API token with read access to the gated teams — deliberately not any one specialist's own credential, since listing issues across a team isn't an action taken as that specialist)")
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	gates, err := watcher.LoadGates(gateConfig)
	if err != nil {
		return err
	}

	poller := watcher.GatePoller{
		Gates:    gates,
		Query:    watcher.LinearIssueQuerier{Token: token},
		Delegate: watcher.LinearDelegator{Log: log},
		Log:      log,
	}
	return poller.PollOnce(context.Background())
}
