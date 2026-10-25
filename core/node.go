package core

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Juniper/apstra-go-sdk/apstra"
)

type NodeInfo struct {
	ID    string
	Label string
}

type SystemInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"-"`
}

func GetNodes(ctx context.Context, client *apstra.TwoStageL3ClosClient, nodeType apstra.NodeType) (*NodeInfo, error) {
	// Create properly typed variables to store the results
	var nodes struct {
		Nodes map[string]NodeInfo
	}

	// Get nodes of a specific type from the blueprint
	client.GetNodes(ctx, nodeType, &nodes)

	// Print the nodes in a more structured way
	for _, node := range nodes.Nodes {
		fmt.Printf("ID: %s\tLabel: %s\n", node.ID, node.Label)
		return &node, nil
	}

	return nil, fmt.Errorf("no nodes of type %s found", nodeType)
}

func GetNodeById(ctx context.Context, client *apstra.TwoStageL3ClosClient, nodeId apstra.ObjectId) (*apstra.SystemNodeInfo, error) {

	node, err := client.GetSystemNodeInfo(ctx, nodeId)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func DefaultLeafGroup(ctx context.Context, client *apstra.TwoStageL3ClosClient) (*apstra.SystemNodeInfo, error) {

	nodes, err := client.GetAllSystemNodeInfos(ctx)
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if node.Role == apstra.SystemRoleRedundancyGroup {
			return &node, nil
		}
	}

	return nil, fmt.Errorf("no default leaf group found")
}

func GetLeafs(ctx context.Context, client *apstra.TwoStageL3ClosClient) ([]SystemInfo, error) {
	rawClient := client.Client()
	blueprintId := client.Id()

	ccrUrl, _ := url.Parse(fmt.Sprintf("/api/blueprints/%s/systems", blueprintId))

	req := apstra.RawJsonRequest{
		Method: http.MethodGet,
		Url:    ccrUrl,
	}

	var resp struct {
		Items []SystemInfo `json:"items"`
	}
	err := rawClient.DoRawJsonTransaction(ctx, req, &resp)
	if err != nil {
		return nil, err
	}

	if len(resp.Items) == 0 {
		// When there are no leaf just return emtpy. Happens when a new blueprint is added
		return []SystemInfo{}, nil
	}
	if len(resp.Items) != 2 {
		return nil, fmt.Errorf("expected 2 leafs, found %d", len(resp.Items))
	}

	leafs := make([]SystemInfo, len(resp.Items))
	for i, item := range resp.Items {
		systemUrl, _ := url.Parse(fmt.Sprintf("/api/systems/%s", item.ID))
		req := apstra.RawJsonRequest{
			Method: http.MethodGet,
			Url:    systemUrl,
		}

		var system struct {
			Status struct {
				Hostname string `json:"hostname"`
			} `json:"status"`
		}
		err := rawClient.DoRawJsonTransaction(ctx, req, &system)
		if err != nil {
			return nil, err
		}

		item.Hostname = system.Status.Hostname
		leafs[i] = item
	}

	return leafs, nil
}
