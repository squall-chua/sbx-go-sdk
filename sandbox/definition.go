package sandbox

import (
	"errors"
	"io"
	"strconv"
)

// Definition is the create spec built from options.
type Definition struct {
	agent         string
	workspaces    []string // each may carry a ":ro" suffix
	name          string
	cpus          int
	memory        string
	profile       string
	template      string
	clone         bool
	publish       []string // -p specs, applied at create time
	kits          []string // --kit refs, applied at create time
	denyNetwork   []string // --deny-network hosts, applied at create time
	staticMCP     []string // --static-mcp server names, fixed at create time
	noShareSkills bool     // --no-share-skills, opt out of the shared skills store
	agentArgs     []string
	stdin         io.Reader
	stdout        io.Writer
	stderr        io.Writer
}

func newDefinition(opts ...Option) *Definition {
	d := &Definition{}
	for _, o := range opts {
		o(d)
	}
	return d
}

// toCreateArgs builds the `sbx create ...` argument vector. Workspaces must already
// be absolute (resolved by the caller in lifecycle.go).
func (d *Definition) toCreateArgs() ([]string, error) {
	if d.agent == "" {
		return nil, errors.New("sandbox: agent is required (WithAgent)")
	}
	if len(d.workspaces) == 0 {
		return nil, errors.New("sandbox: at least one workspace is required (WithWorkspace)")
	}
	args := []string{"create", d.agent}
	args = append(args, d.workspaces...)
	if d.name != "" {
		args = append(args, "--name", d.name)
	}
	if d.cpus > 0 {
		args = append(args, "--cpus", strconv.Itoa(d.cpus))
	}
	if d.memory != "" {
		args = append(args, "--memory", d.memory)
	}
	if d.profile != "" {
		args = append(args, "--profile", d.profile)
	}
	if d.template != "" {
		args = append(args, "--template", d.template)
	}
	for _, spec := range d.publish {
		args = append(args, "-p", spec)
	}
	for _, ref := range d.kits {
		args = append(args, "--kit", absLocal(ref))
	}
	for _, host := range d.denyNetwork {
		args = append(args, "--deny-network", host)
	}
	for _, name := range d.staticMCP {
		args = append(args, "--static-mcp", name)
	}
	if d.noShareSkills {
		args = append(args, "--no-share-skills")
	}
	if d.clone {
		args = append(args, "--clone")
	}
	return args, nil
}

// toRunArgs builds the `sbx run AGENT [WORKSPACE...] [create-flags] [-- AGENT_ARGS]`
// vector for the package-level create-if-missing Run. Workspaces must already be
// absolute (resolved by the caller).
func (d *Definition) toRunArgs() ([]string, error) {
	if d.agent == "" {
		return nil, errors.New("sandbox: agent is required (WithAgent)")
	}
	if len(d.workspaces) == 0 {
		return nil, errors.New("sandbox: at least one workspace is required (WithWorkspace)")
	}
	args := []string{"run", d.agent}
	args = append(args, d.workspaces...)
	if d.name != "" {
		args = append(args, "--name", d.name)
	}
	if d.cpus > 0 {
		args = append(args, "--cpus", strconv.Itoa(d.cpus))
	}
	if d.memory != "" {
		args = append(args, "--memory", d.memory)
	}
	if d.profile != "" {
		args = append(args, "--profile", d.profile)
	}
	if d.template != "" {
		args = append(args, "--template", d.template)
	}
	for _, spec := range d.publish {
		args = append(args, "-p", spec)
	}
	for _, ref := range d.kits {
		args = append(args, "--kit", absLocal(ref))
	}
	for _, host := range d.denyNetwork {
		args = append(args, "--deny-network", host)
	}
	for _, name := range d.staticMCP {
		args = append(args, "--static-mcp", name)
	}
	if d.noShareSkills {
		args = append(args, "--no-share-skills")
	}
	if d.clone {
		args = append(args, "--clone")
	}
	if len(d.agentArgs) > 0 {
		args = append(args, "--")
		args = append(args, d.agentArgs...)
	}
	return args, nil
}
