package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// ListApprovals returns the pending approvals (`sbx policy approval ls
// --json`): governance network requests waiting for a user's answer. They
// appear only on an org-governed host with the feature.network-user-prompts
// setting on. Otherwise the list is empty.
//
// Each entry is returned as raw JSON. Only the empty list has been seen (sbx
// v0.51.0 prints []), so the entry shape is unknown and the SDK does not guess
// one. Output that is not a JSON array, null included, is
// client.ErrUnexpectedFormat.
func ListApprovals(ctx context.Context, c *client.Client) ([]json.RawMessage, error) {
	raw, err := capture(ctx, c, "policy", "approval", "ls", "--json")
	if err != nil {
		return nil, err
	}
	var v *[]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("policy approval ls: %w: %w", client.ErrUnexpectedFormat, err)
	}
	if v == nil {
		return nil, fmt.Errorf("policy approval ls: %w: output is null, not a list", client.ErrUnexpectedFormat)
	}
	return *v, nil
}

// InspectApprovalRaw returns the CLI's human-rendered detail for a pending
// approval (`sbx policy approval inspect ID`), including the option IDs that
// RespondApproval takes. It is raw text because the command has no --json.
// An ID that is not pending is an error from the CLI.
func InspectApprovalRaw(ctx context.Context, c *client.Client, id string) (string, error) {
	if id == "" {
		return "", errors.New("policy approval inspect: id must not be empty")
	}
	return capture(ctx, c, "policy", "approval", "inspect", id)
}

// RespondApproval answers a pending approval with one of the option IDs it
// offers (`sbx policy approval respond ID --option OPTION`). An ID that is not
// pending is an error from the CLI ("approval … is not pending").
func RespondApproval(ctx context.Context, c *client.Client, id, option string) error {
	if id == "" || option == "" {
		return errors.New("policy approval respond: id and option must not be empty")
	}
	return run(ctx, c, "policy", "approval", "respond", id, "--option", option)
}
