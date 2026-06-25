// Package template manages sandbox template images (templates ARE images) over the
// daemon's /docker/images* REST endpoints, plus the shell-out save path on Sandbox.
package template

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/transport"
)

// Image is a template image (base or saved). Listing returns the full set.
type Image struct {
	Agent      string `json:"agent"`
	CreatedAt  string `json:"created_at"`
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

// List returns all template images (REST GET /docker/images).
func List(ctx context.Context, c *client.Client) ([]Image, error) {
	var imgs []Image
	if err := c.Transport().DoJSON(ctx, http.MethodGet, "/docker/images", nil, &imgs); err != nil {
		return nil, client.MapError("template-list", err)
	}
	return imgs, nil
}

// Inspect returns a single image by ref (REST GET /docker/images/inspect?name=).
func Inspect(ctx context.Context, c *client.Client, ref string) (Image, error) {
	var img Image
	path := "/docker/images/inspect?name=" + url.QueryEscape(ref)
	if err := c.Transport().DoJSON(ctx, http.MethodGet, path, nil, &img); err != nil {
		return Image{}, client.MapError("template-inspect", err)
	}
	return img, nil
}

// httpStatus reads+closes resp and returns a transport.HTTPStatusError if the
// status is non-2xx, else nil.
func httpStatus(resp *http.Response) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &transport.HTTPStatusError{Status: resp.StatusCode, Body: body}
	}
	return nil
}

// Remove deletes a template image by ref (tag or id). REST DELETE
// /docker/images/remove?name=<ref>. Wire shape verified live against sandboxd
// v0.33.0 (internal/integration TestSmoke_TemplateSaveRemoveLoad).
func Remove(ctx context.Context, c *client.Client, ref string) error {
	path := "/docker/images/remove?name=" + url.QueryEscape(ref)
	resp, err := c.Transport().Do(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return client.MapError("template-remove", err)
	}
	if err := httpStatus(resp); err != nil {
		return client.MapError("template-remove", err)
	}
	return nil
}

// Load imports an image tar into the runtime image store (REST POST
// /docker/images/load with the tar as the request body). Wire shape verified live
// against sandboxd v0.33.0 (internal/integration TestSmoke_TemplateSaveRemoveLoad).
func Load(ctx context.Context, c *client.Client, tar io.Reader) error {
	resp, err := c.Transport().Do(ctx, http.MethodPost, "/docker/images/load", tar, nil)
	if err != nil {
		return client.MapError("template-load", err)
	}
	if err := httpStatus(resp); err != nil {
		return client.MapError("template-load", err)
	}
	return nil
}
