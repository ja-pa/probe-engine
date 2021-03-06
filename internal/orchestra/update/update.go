// Package update contains code to update the probe state with orchestra
package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/ooni/probe-engine/internal/jsonapi"
	"github.com/ooni/probe-engine/internal/orchestra/login"
	"github.com/ooni/probe-engine/internal/orchestra/metadata"
	"github.com/ooni/probe-engine/log"
)

// Config contains configs for calling the update API.
type Config struct {
	Auth       *login.Auth
	BaseURL    string
	ClientID   string
	HTTPClient *http.Client
	Logger     log.Logger
	Metadata   metadata.Metadata
	UserAgent  string
}

type request struct {
	metadata.Metadata
}

// Do registers this probe with OONI orchestra
func Do(ctx context.Context, config Config) error {
	if config.Auth == nil {
		return errors.New("config.Auth is nil")
	}
	authorization := fmt.Sprintf("Bearer %s", config.Auth.Token)
	req := &request{Metadata: config.Metadata}
	var resp struct{}
	urlpath := fmt.Sprintf("/api/v1/update/%s", config.ClientID)
	return (&jsonapi.Client{
		Authorization: authorization,
		BaseURL:       config.BaseURL,
		HTTPClient:    config.HTTPClient,
		Logger:        config.Logger,
		UserAgent:     config.UserAgent,
	}).Update(ctx, urlpath, req, &resp)
}
