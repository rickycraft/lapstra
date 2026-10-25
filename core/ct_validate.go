package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Juniper/apstra-go-sdk/apstra"
)

type ctExportResponse struct {
	Policies []json.RawMessage `json:"policies"`
}

type ctExportObject struct {
	Visible bool   `json:"visible"`
	Label   string `json:"label"`
}

func GetCtObj(ctx context.Context, client *apstra.TwoStageL3ClosClient) ([]string, error) {
	rawClient := client.Client()
	blueprintId := client.Id()

	ctUrl, _ := url.Parse(fmt.Sprintf("/api/blueprints/%s/obj-policy-export", blueprintId))

	req := apstra.RawJsonRequest{
		Method: http.MethodGet,
		Url:    ctUrl,
	}

	var resp json.RawMessage
	err := rawClient.DoRawJsonTransaction(ctx, req, &resp)
	if err != nil {
		return nil, err
	}

	return CtObjFromObjPolicyExportJSON(resp)
}

func CtObjFromObjPolicyExportJSON(data []byte) ([]string, error) {
	var resp ctExportResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	labels := make([]string, 0)
	for _, rawCt := range resp.Policies {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(rawCt, &raw); err != nil {
			return nil, err
		}
		if _, ok := raw["user_data"]; ok {
			continue
		}

		var ct ctExportObject
		if err := json.Unmarshal(rawCt, &ct); err != nil {
			return nil, err
		}
		if ct.Visible {
			labels = append(labels, ct.Label)
		}
	}

	return labels, nil
}
