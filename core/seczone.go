package core

import (
	"context"
	"fmt"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/Juniper/apstra-go-sdk/datacenter"
	"github.com/Juniper/apstra-go-sdk/enum"
)

func DefaultSecurityZone(ctx context.Context, client *apstra.TwoStageL3ClosClient) (*apstra.ObjectId, error) {
	securityZone, err := client.GetSecurityZoneByVRFName(ctx, "EVPN-Default")
	if err != nil {
		return nil, fmt.Errorf("error getting default security zone - %w", err)
	}
	id := securityZone.ID()
	if id == nil {
		return nil, fmt.Errorf("default security zone has no id")
	}
	objectId := apstra.ObjectId(*id)
	return &objectId, nil
}

func GetSecurityZoneByLabel(ctx context.Context, client *apstra.TwoStageL3ClosClient, label string) (*datacenter.SecurityZone, error) {
	securityZones, err := client.GetSecurityZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting security zones: %w", err)
	}

	for _, securityZone := range securityZones {
		if securityZone.Label == label {
			return &securityZone, nil
		}
	}

	return nil, fmt.Errorf("security zone %q not found", label)
}

func GetSecurityZoneLabelMap(ctx context.Context, client *apstra.TwoStageL3ClosClient) (map[string]*datacenter.SecurityZone, error) {
	securityZones, err := client.GetSecurityZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting security zones: %w", err)
	}

	securityZoneMap := make(map[string]*datacenter.SecurityZone, len(securityZones))
	for idx := range securityZones {
		label := securityZones[idx].Label
		if _, ok := securityZoneMap[label]; !ok {
			securityZoneMap[label] = &securityZones[idx]
		}
	}

	return securityZoneMap, nil
}

func CreateSecurityZone(ctx context.Context, client *apstra.TwoStageL3ClosClient, name string, description string, vid uint16) (apstra.ObjectId, error) {

	vni := 10000000 + int(vid)
	conf := datacenter.SecurityZone{
		Label:            name,
		VRFName:          name,
		VLAN:             &vid,
		VNI:              &vni,
		JunosEVPNIRBMode: &enum.JunosEVPNIRBModeSymmetric,
		Type:             enum.SecurityZoneTypeEVPN,
	}

	id, err := client.CreateSecurityZone(ctx, conf)
	return apstra.ObjectId(id), err
}
