// Package template manages sandbox template images (templates ARE images) over the
// daemon's /docker/images* REST endpoints, plus the shell-out save path on Sandbox.
package template

import (
	"context"
	"net/http"
	"net/url"

	"github.com/squall-chua/sbx-go-sdk/client"
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
