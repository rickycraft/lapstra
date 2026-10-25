package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Juniper/apstra-go-sdk/apstra"
)

// GetConfigContext returns the inner JSON, whether Apstra sends an object or a
// JSON-encoded string, without decoding its numbers.
func GetConfigContext(ctx context.Context, client *apstra.TwoStageL3ClosClient, nodeID apstra.ObjectId) (json.RawMessage, error) {
	endpoint := &url.URL{Path: fmt.Sprintf("/api/blueprints/%s/nodes/%s/config-context", client.Id(), nodeID)}
	var response struct {
		Context json.RawMessage `json:"context"`
	}
	err := client.Client().DoRawJsonTransaction(ctx, apstra.RawJsonRequest{
		Method: http.MethodGet,
		Url:    endpoint,
	}, &response)
	if err != nil {
		return nil, fmt.Errorf("get config context for node %s: %w", nodeID, err)
	}
	// Some Apstra versions wrap the context JSON in a string. Unwrap it before
	// returning the raw object so the renderer can decode numbers with UseNumber.
	raw := bytes.TrimSpace(response.Context)
	if len(raw) > 0 && raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return nil, fmt.Errorf("decode config context string for node %s: %w", nodeID, err)
		}
		raw = bytes.TrimSpace([]byte(encoded))
	}
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, fmt.Errorf("missing or null config context for node %s", nodeID)
	}
	return raw, nil
}
