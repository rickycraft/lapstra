package core

import (
	"context"
	"fmt"
	"net"
	"slices"

	"github.com/IBM/fp-go/array"
	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/Juniper/apstra-go-sdk/datacenter"
)

type StaticRouteSpec struct {
	Network      *net.IPNet
	NextHop      net.IP
	SecurityZone *datacenter.SecurityZone
}

func GetConnectivityTemplateByName(ctx context.Context, client *apstra.TwoStageL3ClosClient, name string) (*apstra.ConnectivityTemplate, error) {
	cts, err := client.GetAllConnectivityTemplates(ctx)
	if err != nil {
		return nil, err
	}

	for _, ct := range cts {
		if ct.Label == name {
			return &ct, nil
		}
	}

	return nil, fmt.Errorf("connectivity template %q not found", name)
}

func GetConnectivityTemplateLabelMap(ctx context.Context, client *apstra.TwoStageL3ClosClient) (map[string]*apstra.ConnectivityTemplate, error) {
	cts, err := client.GetAllConnectivityTemplates(ctx)
	if err != nil {
		return nil, err
	}

	ctMap := make(map[string]*apstra.ConnectivityTemplate, len(cts))
	for idx := range cts {
		label := cts[idx].Label
		if _, ok := ctMap[label]; !ok {
			ctMap[label] = &cts[idx]
		}
	}

	return ctMap, nil
}

func CreateConnectivityTemplate(ctx context.Context, client *apstra.TwoStageL3ClosClient, name string) (*apstra.ConnectivityTemplate, error) {
	ct := &apstra.ConnectivityTemplate{Label: name}
	if err := client.CreateConnectivityTemplate(ctx, ct); err != nil {
		return nil, err
	}

	return ct, nil
}

func AssignConnectivityTemplateToLeafs(ctx context.Context, client *apstra.TwoStageL3ClosClient, ctID apstra.ObjectId, leafs []SystemInfo) error {
	if len(leafs) != 2 {
		return fmt.Errorf("expected 2 leafs, found %d", len(leafs))
	}

	assignments := make(map[apstra.ObjectId]map[apstra.ObjectId]bool, len(leafs))
	for _, leaf := range leafs {
		assignments[apstra.ObjectId(leaf.ID)] = map[apstra.ObjectId]bool{ctID: true}
	}

	return client.SetApplicationPointsConnectivityTemplates(ctx, assignments)
}

func CreateConnectivityTemplateVlans(ctx context.Context, client *apstra.TwoStageL3ClosClient, name string, vns []apstra.ObjectId) (*apstra.ObjectId, error) {

	attributes := apstra.ConnectivityTemplatePrimitiveAttributesAttachMultipleVlan{
		Label:           name + "-Vlan",
		TaggedVnNodeIds: vns,
	}
	ctPrimitive := apstra.ConnectivityTemplatePrimitive{
		Label:      "VlanMultiple-Primitive",
		Attributes: &attributes,
	}

	ct := apstra.ConnectivityTemplate{
		Label:       name,
		Subpolicies: []*apstra.ConnectivityTemplatePrimitive{&ctPrimitive},
	}

	err := client.CreateConnectivityTemplate(ctx, &ct)
	if err != nil {
		return nil, err
	}

	return ct.Id, nil
}

func updateConnectivityTemplateVn(ctx context.Context, client *apstra.TwoStageL3ClosClient, id apstra.ObjectId, vns []apstra.ObjectId, add bool) error {

	ct, err := client.GetConnectivityTemplate(ctx, id)
	if err != nil {
		return fmt.Errorf("error getting connectivity template: %w", err)
	}
	if len(ct.Subpolicies) == 0 {
		return fmt.Errorf("no Subpolicies found in connectivity template %v", id)
	}

	ctPrimitive := ct.Subpolicies[0]
	attributes := ctPrimitive.Attributes
	if attributes.PolicyTypeName() != apstra.CtPrimitivePolicyTypeNameAttachMultipleVlan {
		return fmt.Errorf("connectivity template %v is not a Vlan Attach policy", id)
	}
	vlanAttr := attributes.(*apstra.ConnectivityTemplatePrimitiveAttributesAttachMultipleVlan)
	if add {
		// Add the VNs to the list
		vlanAttr.TaggedVnNodeIds = append(vlanAttr.TaggedVnNodeIds, vns...)
	} else {
		// Remove the VNs from the list
		vlanAttr.TaggedVnNodeIds = array.Filter(func(a apstra.ObjectId) bool {
			return !slices.Contains(vns, a)
		})(vlanAttr.TaggedVnNodeIds)
	}
	ctPrimitive.Attributes = vlanAttr

	ctNew := apstra.ConnectivityTemplate{
		Id:          &id,
		Label:       ct.Label,
		Subpolicies: []*apstra.ConnectivityTemplatePrimitive{ctPrimitive},
	}

	err = client.UpdateConnectivityTemplate(ctx, &ctNew)
	if err != nil {
		return fmt.Errorf("error updating connectivity template: %w", err)
	}

	return nil
}

func updateConnectivityTemplateVxlan(ctx context.Context, client *apstra.TwoStageL3ClosClient, id apstra.ObjectId, vnis []uint32, add bool) error {
	var vns []apstra.ObjectId

	for _, vid := range vnis {
		vn, err := GetVirtualNetworkVni(ctx, client, vid)
		if err != nil {
			return fmt.Errorf("vni %d not found: %w", vid, err)
		}
		vns = append(vns, vn.Id)
	}

	return updateConnectivityTemplateVn(ctx, client, id, vns, add)
}

func UpdateConnectivityTemplateAddVxlan(ctx context.Context, client *apstra.TwoStageL3ClosClient, id apstra.ObjectId, vnis []uint32) error {
	return updateConnectivityTemplateVxlan(ctx, client, id, vnis, true)
}

func UpdateConnectivityTemplateRemoveVxlan(ctx context.Context, client *apstra.TwoStageL3ClosClient, id apstra.ObjectId, vnis []uint32) error {
	return updateConnectivityTemplateVxlan(ctx, client, id, vnis, false)
}

func updateConnectivityTemplateVid(ctx context.Context, client *apstra.TwoStageL3ClosClient, id apstra.ObjectId, vids []uint16, add bool) error {
	var vns []apstra.ObjectId

	for _, vid := range vids {
		vn, err := GetVirtualNetworkVid(ctx, client, vid)
		if err != nil {
			return fmt.Errorf("vid %v not found", vid)
		}
		vns = append(vns, vn.Id)
	}

	return updateConnectivityTemplateVn(ctx, client, id, vns, add)
}

func UpdateConnectivityTemplateAddVid(ctx context.Context, client *apstra.TwoStageL3ClosClient, id apstra.ObjectId, vids []uint16) error {
	return updateConnectivityTemplateVid(ctx, client, id, vids, true)
}

func UpdateConnectivityTemplateRemoveVid(ctx context.Context, client *apstra.TwoStageL3ClosClient, id apstra.ObjectId, vids []uint16) error {
	return updateConnectivityTemplateVid(ctx, client, id, vids, false)
}

func UpdateConnectivityTemplateAddStaticRoutes(ctx context.Context, client *apstra.TwoStageL3ClosClient, ct *apstra.ConnectivityTemplate, routes []StaticRouteSpec) error {
	if len(routes) == 0 {
		return fmt.Errorf("static routes cannot be empty")
	}

	for idx, route := range routes {
		if route.Network == nil {
			return fmt.Errorf("static route %d network cannot be nil", idx)
		}
		if route.NextHop == nil {
			return fmt.Errorf("static route %d next-hop cannot be nil", idx)
		}
		if route.SecurityZone == nil {
			return fmt.Errorf("static route %d security zone cannot be nil", idx)
		}

		id := route.SecurityZone.ID()
		if id == nil {
			return fmt.Errorf("static route %d security zone has no id", idx)
		}
		securityZoneId := apstra.ObjectId(*id)
		staticRouteAttr := apstra.ConnectivityTemplatePrimitiveAttributesAttachCustomStaticRoute{
			Label:        "CustomStaticRoute",
			Network:      route.Network,
			NextHop:      route.NextHop,
			SecurityZone: &securityZoneId,
		}
		staticRouteLabel := fmt.Sprintf("%s %s", route.SecurityZone.Label, route.Network.String())
		staticRoutePrimitive := apstra.ConnectivityTemplatePrimitive{
			Label:      staticRouteLabel,
			Attributes: &staticRouteAttr,
		}
		ct.Subpolicies = append(ct.Subpolicies, &staticRoutePrimitive)
	}

	ctNew := apstra.ConnectivityTemplate{
		Id:          ct.Id,
		Label:       ct.Label,
		Subpolicies: ct.Subpolicies,
		Description: ct.Description,
	}

	err := client.UpdateConnectivityTemplate(ctx, &ctNew)
	if err != nil {
		return fmt.Errorf("error updating connectivity template: %w", err)
	}

	return nil
}
