package cmd

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/Juniper/apstra-go-sdk/enum"
	"github.com/charmbracelet/huh/spinner"
	"rickycraft/lapstra/core"
)

func (l *LapstraCmd) MapVidVni(idx int, vid uint16) (uint32, error) {
	// Check if vid should be stretched
	location := l.locations[idx]
	if l.IsStretch() {
		// Share the VNI pool between DC1 and DC2.
		if location == DC2 {
			location = DC1
		}
		// Share the VNI pool between DC3 and DC4.
		if location == DC4 {
			location = DC3
		}
	}

	// Find base vni for the blueprint
	dcName := core.MapLocationVnPoolName(location.String())
	pool, err := l.client.GetVniPoolByName(l.ctx, dcName)
	if err != nil {
		return 0, err
	}
	baseVni := pool.Ranges[0].First

	// Map the vid to a vni
	vni := baseVni + uint32(vid)

	return uint32(vni), nil
}

func (l *LapstraCmd) VlanCreateL2() error {

	vid, err := AskVlan()
	if err != nil {
		return err
	}

	name, err := AskName("Virtual network name")
	if err != nil {
		return err
	}

	// Ask for description
	description, err := AskName("Virtual network description")
	if err != nil {
		return err
	}

	vns := make([]*apstra.ObjectId, l.size)
	errors := make([]error, l.size)
	wg := sync.WaitGroup{}
	for idx, client := range l.clients {
		wg.Go(func() {
			vni, err := l.MapVidVni(idx, vid)
			if err != nil {
				errors[idx] = err
				return
			}
			vn, err := core.CreateVnL2WithBinding(l.ctx, client, name, vni, vid, description)
			if err != nil {
				errors[idx] = err
				return
			}
			l.PrintCommandResult(idx,
				"created %s vlan %d vni %d", name, vid, vni,
			)
			vns[idx] = vn
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	l.PrintCommandErrors(errors)

	return nil
}

func (l *LapstraCmd) VlanCreateL3() error {

	vid, err := AskVlan()
	if err != nil {
		return err
	}

	name, err := AskName("Virtual network name")
	if err != nil {
		return err
	}

	// Ask for description
	description, err := AskName("Virtual network description")
	if err != nil {
		return err
	}

	ip, network, err := AskIpSubnet("Virtual network anycast ip")
	if err != nil {
		return err
	}

	zones, err := l.AskRoutingZone("Select Routing Zone")
	if err != nil {
		return err
	}

	vns := make([]*apstra.ObjectId, l.size)
	errors := make([]error, l.size)
	wg := sync.WaitGroup{}
	for idx, client := range l.clients {
		wg.Go(func() {
			vni, err := l.MapVidVni(idx, vid)
			if err != nil {
				errors[idx] = err
				return
			}
			if zones[idx] == nil {
				errors[idx] = fmt.Errorf("routing zone not found")
				return
			}
			id := zones[idx].ID()
			if id == nil {
				errors[idx] = fmt.Errorf("routing zone %q has no id", zones[idx].Label)
				return
			}
			zoneId := apstra.ObjectId(*id)
			data := core.CreateVnL3{
				Name:        name,
				Vid:         vid,
				Vni:         vni,
				Zone:        zoneId,
				Net:         network,
				Ip:          ip,
				Description: description,
			}
			vn, err := core.CreateVnL3WithBinding(l.ctx, client, data)
			if err != nil {
				errors[idx] = err
				return
			}
			l.PrintCommandResult(idx,
				"created %s vlan %d vni %d ip %s/%d vrf %s", name, vid, vni, ip, NetowrkCIDR(network), zones[idx].Label,
			)
			vns[idx] = vn
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range vns {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "created vn with id %s", vns[idx])
		}
	}

	return nil
}

func (l *LapstraCmd) TransformVlanL3() error {

	vni, err := AskVni()
	if err != nil {
		return err
	}

	ip, network, err := AskIpSubnet("Virtual network anycast ip")
	if err != nil {
		return err
	}

	zones, err := l.AskRoutingZone("Select Routing Zone")
	if err != nil {
		return err
	}
	errors := make([]error, l.size)
	wg := sync.WaitGroup{}
	for idx, client := range l.clients {
		wg.Go(func() {
			vn, err := core.GetVirtualNetworkVni(l.ctx, client, vni)
			if err != nil {
				errors[idx] = err
				return
			}
			if vn.Data == nil {
				errors[idx] = fmt.Errorf("virtual network %s has nil data", vn.Id)
				return
			}
			if vn.Data.VnId == nil {
				errors[idx] = fmt.Errorf("virtual network %s has no VNI", vn.Data.Label)
				return
			}
			if zones[idx] == nil {
				errors[idx] = fmt.Errorf("routing zone not found")
				return
			}
			id := zones[idx].ID()
			if id == nil {
				errors[idx] = fmt.Errorf("routing zone %q has no id", zones[idx].Label)
				return
			}

			data := core.CreateVnL3{
				Vni:  vni,
				Zone: apstra.ObjectId(*id),
				Net:  network,
				Ip:   ip,
			}
			_, err = core.TransformVirtualNetworkL3(l.ctx, client, vn, data)
			if err != nil {
				errors[idx] = err
				return
			}
			l.PrintCommandResult(idx,
				"transformed %s vni %d ip %s/%d vrf %s", vn.Data.Label, vni, ip, NetowrkCIDR(network), zones[idx].Label,
			)
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	l.PrintCommandErrors(errors)

	return nil
}

func (l *LapstraCmd) VlanDelete() error {

	vid, err := AskVlan()
	if err != nil {
		return err
	}

	vns := make([]*apstra.VirtualNetwork, l.size)
	wg := sync.WaitGroup{}
	for idx, client := range l.clients {
		wg.Go(func() {
			vn, err := core.GetVirtualNetworkVid(l.ctx, client, vid)
			if err == nil {
				vns[idx] = vn
			}
		})
	}
	spinner.New().Action(func() { wg.Wait() }).Run()

	return l.vnDelete(vns)
}

func (l *LapstraCmd) VlanSearch() error {

	vid, err := AskVlan()
	if err != nil {
		return err
	}

	vns := make([]*apstra.VirtualNetwork, l.size)
	ctLabels := make([][]string, l.size)
	errors := make([]error, l.size)
	wg := sync.WaitGroup{}
	ctx, canc := l.Context()
	defer canc()
	for idx, client := range l.clients {
		wg.Go(func() {
			vn, err := core.GetVirtualNetworkVid(ctx, client, vid)
			if err != nil {
				errors[idx] = err
				return
			}
			cts, err := client.GetAllConnectivityTemplates(ctx)
			if err != nil {
				errors[idx] = err
				return
			}
			for _, ct := range cts {
				if len(ct.Subpolicies) == 0 {
					continue
				}
				if ct.Subpolicies[0].Attributes.PolicyTypeName() != apstra.CtPrimitivePolicyTypeNameAttachMultipleVlan {
					continue
				}
				taggedVns := ct.Subpolicies[0].Attributes.(*apstra.ConnectivityTemplatePrimitiveAttributesAttachMultipleVlan).
					TaggedVnNodeIds
				if slices.Contains(taggedVns, vn.Id) {
					ctLabels[idx] = append(ctLabels[idx], ct.Label)
				}
			}
			vns[idx] = vn
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range vns {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "virtual network %s vni %d", vns[idx].Data.Label, *vns[idx].Data.VnId)
			if len(ctLabels[idx]) == 0 {
				l.PrintCommandResult(idx, "not present in any connectivity template")
			} else {
				l.PrintCommandResult(idx, "connectivity template: %s", strings.Join(ctLabels[idx], ", "))
			}
		}
	}

	return nil
}

func (l *LapstraCmd) VlanShow() error {

	vns := make(map[uint32][]*apstra.VirtualNetwork)
	errors := l.Errors()
	wg := sync.WaitGroup{}
	mut := sync.Mutex{}
	ctx, canc := l.Context()
	defer canc()
	for idx, client := range l.clients {
		wg.Go(func() {
			vn, err := client.GetAllVirtualNetworks(ctx)
			if err != nil {
				errors[idx] = err
				return
			}
			mut.Lock()
			defer mut.Unlock()
			for _, v := range vn {
				if v.Data.VnType != enum.VnTypeVxlan {
					continue
				}
				if len(vns[*v.Data.VnId]) == 0 {
					vns[*v.Data.VnId] = make([]*apstra.VirtualNetwork, l.size)
				}
				vns[*v.Data.VnId][idx] = &v
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	results := make([]string, 0)
	for vni, vn := range vns {
		var vid uint16
		line := fmt.Sprintf("VNI %d", vni)
		for idx, v := range vn {
			if v == nil {
				continue
			}
			for _, binding := range v.Data.VnBindings {
				vid = *binding.VlanId
			}
			label := l.labels[idx]
			line = line + fmt.Sprintf("\n\t[%s]\t%s\tVid %d", label, v.Data.Label, vid)
		}
		results = append(results, line)
	}
	sort.Strings(results)
	err := ShowViewport(strings.Join(results, "\n"))
	if err != nil {
		return err
	}

	l.PrintCommandErrors(errors)

	return nil
}
