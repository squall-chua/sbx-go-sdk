package client

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/squall-chua/sbx-go-sdk/internal/transport"
)

func TestMapHTTPError_NotFound(t *testing.T) {
	err := mapHTTPError("inspect", &transport.HTTPStatusError{Status: 404, Body: []byte(`{"message":"sandbox not found"}`)})
	require.ErrorIs(t, err, ErrSandboxNotFound)
	var ae *APIError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, 404, ae.Status)
	require.Equal(t, "sandbox not found", ae.Message)
}

func TestMapHTTPError_PassthroughNon404(t *testing.T) {
	err := mapHTTPError("x", &transport.HTTPStatusError{Status: 500, Body: []byte(`{"message":"boom"}`)})
	require.False(t, errors.Is(err, ErrSandboxNotFound))
	var ae *APIError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, 500, ae.Status)
}
