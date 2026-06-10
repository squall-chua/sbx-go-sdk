package sandbox

import (
	"errors"
	"strconv"
)

// Definition is the create spec built from options.
type Definition struct {
	agent      string
	workspaces []string // each may carry a ":ro" suffix
	name       string
	cpus       int
	memory     string
	profile    string
	template   string
	clone      bool
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
	if d.clone {
		args = append(args, "--clone")
	}
	return args, nil
}
