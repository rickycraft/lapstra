package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Juniper/apstra-go-sdk/apstra"
)

// ConnectToApstra establishes a connection to the Apstra API.
func ConnectToApstra(ctx context.Context, apiEndpoint string, username string, password string) (*apstra.Client, error) {
	// Create a new Apstra client
	clientCfg := &apstra.ClientCfg{
		Url:        apiEndpoint,
		User:       username,
		Pass:       password,
		HttpClient: &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}},
		Timeout:    1 * time.Minute,
	}
	client, err := clientCfg.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func GetAllBlueprints(ctx context.Context, client *apstra.Client) ([]apstra.ObjectId, error) {

	blueprints, err := client.GetAllBlueprintStatus(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]apstra.ObjectId, 0)
	for _, blueprint := range blueprints {
		ids = append(ids, blueprint.Id)
	}

	return ids, nil
}

func GetBlueprintPrefix(ctx context.Context, client *apstra.Client, prefix string) ([]apstra.ObjectId, error) {

	blueprints, err := client.GetAllBlueprintStatus(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]apstra.ObjectId, 0)
	for _, blueprint := range blueprints {
		if strings.Contains(blueprint.Label, prefix) {
			ids = append(ids, blueprint.Id)
		}
	}

	return ids, nil
}

// ConnectToApstra establishes a connection to the Apstra API with a specific blueprint.
func ConnectToBlueprint(ctx context.Context, client *apstra.Client, blueprintName string) (*apstra.TwoStageL3ClosClient, error) {
	blueprint, err := client.GetBlueprintStatusByName(ctx, blueprintName)
	if err != nil {
		return nil, fmt.Errorf("failed to get blueprint: %w", err)
	}

	dcClient, err := client.NewTwoStageL3ClosClient(ctx, blueprint.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to create TwoStageL3ClosClient: %w", err)
	}

	return dcClient, nil
}
