package core

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Juniper/apstra-go-sdk/apstra"
)

type BlueprintState string

const (
	StatePending     BlueprintState = "pending"
	StateSuccess     BlueprintState = "success"
	StateRejected    BlueprintState = "rejected"
	StateUnloadable  BlueprintState = "unloadable"
	StateUnavailable BlueprintState = "unavailable"
	StateFailure     BlueprintState = "failure"
	StateEmpty       BlueprintState = "empty"
	StateUnknown     BlueprintState = "unknown"
	StateTimeout     BlueprintState = "timeout"
	StateUnsupported BlueprintState = "unsupported"
)

type Validity string

const (
	ValidityStale   Validity = "stale"
	ValidityUnknown Validity = "unknown"
	ValidityFresh   Validity = "fresh"
)

type BlueprintCommitCheckResult struct {
	State            BlueprintState `json:"state"`
	BlueprintVersion int            `json:"blueprint_version,omitempty"`
	Validity         Validity       `json:"validity"`
}

type CommitCheckResult struct {
	Blueprint BlueprintCommitCheckResult `json:"blueprint"`
}

type SystemCommitCheckResult struct {
	Diff             string   `json:"diff_string"`
	BlueprintVersion int      `json:"blueprint_version,omitempty"`
	Validity         Validity `json:"validity"`
	Error            string   `json:"error"`
}

func StartCommitCheck(ctx context.Context, client *apstra.TwoStageL3ClosClient) error {

	rawClient := client.Client()
	blueprintId := client.Id()

	ccrUrl, _ := url.Parse(fmt.Sprintf("/api/blueprints/%s/commit-check", blueprintId))

	req := apstra.RawJsonRequest{
		Method: http.MethodPost,
		Url:    ccrUrl,
	}

	var resp struct{}

	err := rawClient.DoRawJsonTransaction(ctx, req, &resp)
	if err != nil {
		return err
	}

	return nil
}

func GetCommitCheck(ctx context.Context, client *apstra.TwoStageL3ClosClient) (*BlueprintCommitCheckResult, error) {

	rawClient := client.Client()
	blueprintId := client.Id()

	ccrUrl, _ := url.Parse(fmt.Sprintf("/api/blueprints/%s/commit-check-result", blueprintId))

	req := apstra.RawJsonRequest{
		Method: http.MethodGet,
		Url:    ccrUrl,
	}

	var resp CommitCheckResult

	err := rawClient.DoRawJsonTransaction(ctx, req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp.Blueprint, nil
}

func GetDeviceDiff(ctx context.Context, client *apstra.TwoStageL3ClosClient, systemId string) (*SystemCommitCheckResult, error) {

	rawClient := client.Client()
	blueprintId := client.Id()

	ccrUrl, _ := url.Parse(fmt.Sprintf("/api/blueprints/%s/systems/%s/commit-check-result", blueprintId, systemId))

	req := apstra.RawJsonRequest{
		Method: http.MethodGet,
		Url:    ccrUrl,
	}

	var resp SystemCommitCheckResult
	err := rawClient.DoRawJsonTransaction(ctx, req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func DeployBlueprint(ctx context.Context, client *apstra.TwoStageL3ClosClient, description string) error {

	rawClient := client.Client()
	blueprintId := client.Id()

	commitResult, err := GetCommitCheck(ctx, client)
	if err != nil {
		return err
	}

	if commitResult.State != StateSuccess && commitResult.Validity != ValidityFresh {
		return fmt.Errorf("blueprint is not in state success fresh")
	}

	deployRequest := apstra.BlueprintDeployRequest{
		Id:          blueprintId,
		Description: description,
		Version:     commitResult.BlueprintVersion,
	}
	res, err := rawClient.DeployBlueprint(ctx, &deployRequest)
	if err != nil {
		return err
	}
	if res.Error != nil {
		return fmt.Errorf("deploy failed: %x", *res.Error)
	}

	return nil
}

func RevertBlueprint(ctx context.Context, client *apstra.TwoStageL3ClosClient) error {

	rawClient := client.Client()
	blueprintId := client.Id()

	revertUrl, _ := url.Parse(fmt.Sprintf("/api/blueprints/%s/revert", blueprintId))

	req := apstra.RawJsonRequest{
		Method: http.MethodPost,
		Url:    revertUrl,
	}

	return rawClient.DoRawJsonTransaction(ctx, req, nil)
}
