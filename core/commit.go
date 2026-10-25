package core

import (
	"context"

	"github.com/Juniper/apstra-go-sdk/apstra"
)

func Commit(ctx context.Context, client *apstra.Client, id apstra.ObjectId, commitMsg string) (*apstra.BlueprintDeployResponse, error) {
	// TODO: check blueprint status

	// Get the latest commit
	revisions, err := client.GetRevisions(ctx, id)
	if err != nil {
		return nil, err
	}
	latestRevision := revisions[0].RevisionId
	for _, revision := range revisions {
		if revision.RevisionId > latestRevision {
			latestRevision = revision.RevisionId
		}
	}

	// Commit the changes
	commitReq := apstra.BlueprintDeployRequest{
		Description: commitMsg,
		Version:     latestRevision,
	}
	res, err := client.DeployBlueprint(ctx, &commitReq)
	if err != nil {
		return nil, err
	}

	return res, nil
}
