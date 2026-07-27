package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// absLocal makes ref absolute when it names something that exists on disk,
// and returns it untouched otherwise.
//
// The daemon records a sandbox's kit list verbatim and re-resolves the whole
// list on every later add, resolving a relative path against its OWN working
// directory rather than the caller's. The add still reports success and the
// kit is applied, so the damage is invisible until a later, unrelated add on
// the same sandbox fails with "re-resolve original kit 0 (...): path does not
// exist" — after which that sandbox can take no kits at all. Verified
// 2026-07-27 against sbx v0.37.0 for both `kit add` and `create --kit`.
//
// The stat is a fact check, not a guess at reference grammar: an OCI
// reference or a git URL does not stat, so it passes through and the CLI
// stays the authority on what it is.
func absLocal(ref string) string {
	if _, err := os.Stat(ref); err != nil {
		return ref
	}
	abs, err := filepath.Abs(ref)
	if err != nil {
		return ref
	}
	return abs
}

// AddKit adds a kit artifact to an existing sandbox (`sbx kit add`).
//
// ref may be a local directory, a ZIP file, a git repository, or an OCI
// reference. A local path is made absolute first; see absLocal for why.
//
// The container is recreated with the kit appended to the sandbox's kit list.
// Kit-owned volumes, such as agent session state, survive the swap.
// Bind-mounted workspaces keep their host mount, and --clone sandboxes keep
// their in-container tree via a named workspace volume that reattaches.
//
// AddKit applies only part of a kit. The CLI refuses, before touching
// anything, any kit declaring a field the recreate flow does not implement:
//
//	applied: environment.variables, caps.network, commands.install, agentContext
//	refused: commands.startup, commands.initFiles, credentials
//	         (including environment.proxyManaged), publishedPorts, volumes
//
// The remedy in each case is to recreate the sandbox with WithKit, which the
// CLI's own message names. Verified 2026-07-27 against sbx v0.37.0; the list
// is worded "does not yet" upstream and is expected to shrink.
//
// The CLI also refuses a sandbox using a legacy git worktree, and one created
// before the kit-add recreate feature shipped. Neither refusal is classified
// here: both arrive as the CLI's own error, carrying its explanation.
func (s *Sandbox) AddKit(ctx context.Context, ref string) error {
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "kit", "add", s.info.Name, absLocal(ref))
	return err
}

// kitsLabel is where the daemon records a sandbox's kit list.
const kitsLabel = "com.docker.sandbox.kits"

// Kits returns the kit references recorded on the sandbox, in the order they
// were added.
//
// The list is read from the com.docker.sandbox.kits label, which the daemon
// writes as a JSON string array; SandboxInfo has no kit field of its own.
// Verified 2026-07-27 against sandboxd 0.24.0.
//
// Kits refreshes the sandbox first. The handle's cached info is a snapshot
// taken at Get or Create, and AddKit recreates the container, so a cached
// read would report the list as it was before the add.
//
// A sandbox with no kits returns an empty slice. If upstream renames the
// label, this returns empty rather than an error — a silent answer, and a
// known risk of reading a label instead of a documented field.
func (s *Sandbox) Kits(ctx context.Context) ([]string, error) {
	info, err := s.Inspect(ctx)
	if err != nil {
		return nil, err
	}
	if info.Labels == nil {
		return nil, nil
	}
	raw, ok := (*info.Labels)[kitsLabel]
	if !ok || raw == "" {
		return nil, nil
	}
	var kits []string
	if err := json.Unmarshal([]byte(raw), &kits); err != nil {
		return nil, fmt.Errorf("%w: %s label: %v", client.ErrUnexpectedFormat, kitsLabel, err)
	}
	return kits, nil
}
