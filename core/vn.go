package core

import (
	"context"
	"fmt"
	"net"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/Juniper/apstra-go-sdk/enum"
)

func PrintVirtualNetwork(client *apstra.TwoStageL3ClosClient, network *apstra.VirtualNetwork) {

	// Print each virtual network
	fmt.Printf("Virtual Network ID: %s\n", network.Id)
	// fmt.Printf("\tData: %v\n", network.Data)
	fmt.Printf("\tLabel: %v\n", network.Data.Label)
	// fmt.Printf("\tVnType: %v\n", network.Data.VnType)
	fmt.Printf("\tVnId: %d\n", *network.Data.VnId)
	fmt.Printf("\tIpv4Enabled: %v\n", network.Data.Ipv4Enabled)
	sz, _ := client.GetSecurityZone(context.Background(), string(network.Data.SecurityZoneId))
	vrfName := sz.Label
	fmt.Printf("\tSecurityZone: %s / %s\n", network.Data.SecurityZoneId, vrfName)
	fmt.Printf("\tRouteTarget: %v\n", network.Data.RouteTarget)
	fmt.Println("\tVnBindings:")
	for _, binding := range network.Data.VnBindings {
		node, _ := GetNodeById(context.Background(), client, binding.SystemId)
		fmt.Printf("\t\tSystem: %s / %s, VlanId: %d\n", binding.SystemId, node.Label, *binding.VlanId)
	}
}

func GetVirtualNetworkVid(ctx context.Context, client *apstra.TwoStageL3ClosClient, vlan uint16) (*apstra.VirtualNetwork, error) {

	networks, err := client.GetAllVirtualNetworks(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting virtual networks: %v", err)
	}
	if len(networks) == 0 {
		return nil, fmt.Errorf("no virtual networks found")
	}

	for _, network := range networks {
		for _, binding := range network.Data.VnBindings {
			if *binding.VlanId == vlan {
				return &network, nil
			}
		}
	}

	return nil, fmt.Errorf("virtual network with vid %d not found", vlan)
}

func GetVirtualNetworkVni(ctx context.Context, client *apstra.TwoStageL3ClosClient, vnid uint32) (*apstra.VirtualNetwork, error) {

	networks, err := client.GetAllVirtualNetworks(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting virtual networks: %v", err)
	}
	if len(networks) == 0 {
		return nil, fmt.Errorf("no virtual networks found")
	}

	for _, network := range networks {
		if *network.Data.VnId == vnid {
			return &network, nil
		}
	}

	return nil, fmt.Errorf("virtual network with vni %d not found", vnid)
}

func GetVirtualNetworks(ctx context.Context, client *apstra.TwoStageL3ClosClient, ids []apstra.ObjectId) ([]apstra.VirtualNetwork, error) {

	networks, err := client.GetAllVirtualNetworks(ctx)
	if err != nil {
		return nil, err
	}
	if len(networks) == 0 {
		return []apstra.VirtualNetwork{}, nil
	}

	result := make([]apstra.VirtualNetwork, len(ids))
	for idx, id := range ids {
		result[idx] = networks[id]
	}

	return result, nil
}

func CreateVirtualNetworkL2(ctx context.Context, client *apstra.TwoStageL3ClosClient, name string, vni uint32, description string) (*apstra.ObjectId, error) {

	securityZoneId, err := DefaultSecurityZone(ctx, client)
	if securityZoneId == nil {
		return nil, err
	}

	vn := apstra.VirtualNetworkData{
		VnType:         enum.VnTypeVxlan,
		Label:          name,
		SecurityZoneId: *securityZoneId,
		VnId:           &vni,
		Ipv4Enabled:    false,
		Description:    description,
	}
	netId, err := client.CreateVirtualNetwork(ctx, &vn)
	if err != nil {
		return nil, fmt.Errorf("error creating virtual network: %v", err)
	}

	return &netId, nil
}

type CreateVnL3 struct {
	Name        string
	Vid         uint16
	Vni         uint32
	Zone        apstra.ObjectId
	Net         *net.IPNet
	Ip          net.IP
	Description string
}

func CreateVirtualNetworkL3(ctx context.Context, client *apstra.TwoStageL3ClosClient, data CreateVnL3) (*apstra.ObjectId, error) {

	vn := apstra.VirtualNetworkData{
		VnType:                    enum.VnTypeVxlan,
		Label:                     data.Name,
		SecurityZoneId:            data.Zone,
		VnId:                      &data.Vni,
		Ipv4Enabled:               true,
		VirtualGatewayIpv4Enabled: true,
		Ipv4Subnet:                data.Net,
		VirtualGatewayIpv4:        data.Ip,
		Description:               data.Description,
	}
	netId, err := client.CreateVirtualNetwork(ctx, &vn)
	if err != nil {
		return nil, fmt.Errorf("error creating virtual network: %v", err)
	}

	return &netId, nil
}

func TransformVirtualNetworkL3(ctx context.Context, client *apstra.TwoStageL3ClosClient, vn *apstra.VirtualNetwork, data CreateVnL3) (*apstra.ObjectId, error) {

	if vn == nil {
		return nil, fmt.Errorf("virtual network is nil")
	}
	if vn.Data == nil {
		return nil, fmt.Errorf("virtual network %s has nil data", vn.Id)
	}

	vn.Data.SecurityZoneId = data.Zone
	vn.Data.Ipv4Enabled = true
	vn.Data.VirtualGatewayIpv4Enabled = true
	vn.Data.Ipv4Subnet = data.Net
	vn.Data.VirtualGatewayIpv4 = data.Ip
	vn.Data.Tags = nil

	err := client.UpdateVirtualNetwork(ctx, vn.Id, vn.Data)
	if err != nil {
		return nil, fmt.Errorf("error transforming virtual network to L3: %v", err)
	}

	return &vn.Id, nil
}

func CreateVirtualNetworkBinding(ctx context.Context, client *apstra.TwoStageL3ClosClient, virtualNetworkId apstra.ObjectId, vlanId uint16) error {

	node, err := DefaultLeafGroup(ctx, client)
	if err != nil {
		return err
	}
	nodeId := node.Id
	dhcpEnabled := apstra.DhcpServiceEnabled(false)
	binding := apstra.VirtualNetworkBindingsRequest{
		VnId:               virtualNetworkId,
		DhcpServiceEnabled: &dhcpEnabled,
		VnBindings: map[apstra.ObjectId]*apstra.VnBinding{
			nodeId: {
				SystemId: nodeId,
				VlanId:   &vlanId,
			},
		},
	}

	err = client.SetVirtualNetworkLeafBindings(ctx, binding)
	if err != nil {
		return fmt.Errorf("error creating virtual network binding: %v", err)
	}

	return nil
}

func CreateVnL2WithBinding(ctx context.Context, client *apstra.TwoStageL3ClosClient, name string, vni uint32, vlanId uint16, description string) (*apstra.ObjectId, error) {

	vn, err := CreateVirtualNetworkL2(ctx, client, name, vni, description)
	if err != nil {
		return nil, err
	}

	err = CreateVirtualNetworkBinding(ctx, client, *vn, vlanId)
	if err != nil {
		return nil, err
	}

	return vn, nil
}

func CreateVnL3WithBinding(ctx context.Context, client *apstra.TwoStageL3ClosClient, data CreateVnL3) (*apstra.ObjectId, error) {

	vn, err := CreateVirtualNetworkL3(ctx, client, data)
	if err != nil {
		return nil, err
	}

	err = CreateVirtualNetworkBinding(ctx, client, *vn, data.Vid)
	if err != nil {
		return nil, err
	}

	return vn, nil
}

func MapLocationVnPoolName(location string) string {
	return fmt.Sprintf("%s-VNI-POOL", location)
}
