package cmd

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"rickycraft/lapstra/core"
)

// Ask the user to select a CT filtered by the one with multiple vlans
func (l *LapstraCmd) ctMultiVlanSelect() ([]*apstra.ConnectivityTemplate, error) {

	cts := make(map[string][]*apstra.ConnectivityTemplate)
	errors := make([]error, l.size)
	wg := sync.WaitGroup{}
	mut := sync.Mutex{}
	for idx, client := range l.clients {
		wg.Go(func() {
			ct, err := client.GetAllConnectivityTemplates(l.ctx)
			if err != nil {
				errors[idx] = err
				return
			}
			mut.Lock()
			defer mut.Unlock()
			for _, c := range ct {
				if len(c.Subpolicies) == 0 {
					continue
				}
				if c.Subpolicies[0].Attributes.PolicyTypeName() != apstra.CtPrimitivePolicyTypeNameAttachMultipleVlan {
					continue
				}
				if len(cts[c.Label]) == 0 {
					cts[c.Label] = make([]*apstra.ConnectivityTemplate, l.size)
				}
				cts[c.Label][idx] = &c
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()
	l.PrintCommandErrors(errors)

	nameOptions := make([]huh.Option[string], 0)
	var selectedCtName string
	for name := range cts {
		nameOptions = append(nameOptions, huh.Option[string]{
			Value: name,
			Key:   name,
		})
	}
	err := huh.NewSelect[string]().
		Title("Select Connectivity Template").
		Options(nameOptions...).
		Value(&selectedCtName).
		Height(10).
		Run()
	if err != nil {
		return nil, err
	}
	core.Logger().Infof("Selected CT: %s", selectedCtName)

	return cts[selectedCtName], nil
}

func (l *LapstraCmd) CtMultiVlanCreate() error {

	name, err := AskName("Connectivity Template name")
	if err != nil {
		return err
	}

	cts := make([]*apstra.ObjectId, l.size)
	errors := make([]error, l.size)
	wg := sync.WaitGroup{}
	for idx, client := range l.clients {
		wg.Go(func() {
			id, err := core.CreateConnectivityTemplateVlans(l.ctx, client, name, []apstra.ObjectId{})
			if err != nil {
				errors[idx] = err
			} else {
				cts[idx] = id
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range cts {
		if errors[idx] != nil {
			l.PrintCommandError(idx, fmt.Errorf("ct not created - %w", errors[idx]))
		} else {
			l.PrintCommandResult(idx, "created ct %s - %s", name, cts[idx])
		}
	}

	return nil
}

func (l *LapstraCmd) CtMultiVlanRemove() error {
	selectedCts, err := l.ctMultiVlanSelect()
	if err != nil {
		return err
	}

	errors := l.Errors()
	wg := sync.WaitGroup{}
	for idx, ct := range selectedCts {
		if ct == nil {
			continue
		}
		wg.Go(func() {
			err := l.clients[idx].DeleteConnectivityTemplate(l.ctx, *ct.Id)
			if err != nil {
				errors[idx] = err
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx, ct := range selectedCts {
		if ct == nil {
			l.PrintCommandError(idx, fmt.Errorf("ct not found"))
		} else if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "removed ct %s", ct.Label)
		}
	}
	return nil
}

func (l *LapstraCmd) CtVlanSearch() error {

	selectedCts, err := l.ctMultiVlanSelect()
	if err != nil {
		return err
	}
	ctx := context.Background()
	for idx, ct := range selectedCts {
		if ct == nil {
			continue
		}
		vns := ct.Subpolicies[0].Attributes.(*apstra.ConnectivityTemplatePrimitiveAttributesAttachMultipleVlan).
			TaggedVnNodeIds
		vnList, err := core.GetVirtualNetworks(ctx, l.clients[idx], vns)
		if err != nil {
			l.PrintCommandError(idx, err)
			continue
		}
		// Order them consistently
		vnNames := make([]string, len(vnList))
		for i, vn := range vnList {
			vnNames[i] = fmt.Sprintf("%s (%d)", vn.Data.Label, *vn.Data.VnId)
		}
		l.PrintCommandResult(idx, "%s", strings.Join(vnNames, " / "))
	}

	return nil
}

func (l *LapstraCmd) CtVxlanAdd() error {

	cts, err := l.ctMultiVlanSelect()
	if err != nil {
		return err
	}

	vnis, err := AskVnis()
	if err != nil {
		return err
	}
	core.Logger().Infof("Selected VNIs: %v", vnis)

	errors := make([]error, l.size)
	wg := sync.WaitGroup{}
	for idx, ct := range cts {
		if ct == nil {
			continue
		}
		if ct.Id == nil {
			errors[idx] = fmt.Errorf("add vnis %v: ct %q has no id", vnis, ct.Label)
			continue
		}
		client := l.clients[idx]
		ctID := *ct.Id
		ctLabel := ct.Label
		wg.Go(func() {
			l.BlueprintLogger(idx).Debugf("adding vnis %v to CT %q (%s)", vnis, ctLabel, ctID)
			err := core.UpdateConnectivityTemplateAddVxlan(l.ctx, client, ctID, vnis)
			if err != nil {
				errors[idx] = fmt.Errorf("add vnis %v to CT %q (%s): %w", vnis, ctLabel, ctID, err)
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx, ct := range cts {
		if ct == nil {
			l.PrintCommandError(idx, fmt.Errorf("add vnis %v: ct not found", vnis))
		} else if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "added vnis %v to CT %s", vnis, ct.Label)
		}
	}

	return nil
}

func (l *LapstraCmd) CtVxlanRemove() error {

	cts, err := l.ctMultiVlanSelect()
	if err != nil {
		return err
	}

	// Extract all VNI ids from the selected CTs.
	wg := sync.WaitGroup{}
	mut := sync.Mutex{}
	vnis := map[uint32]struct{}{}
	for idx, ct := range cts {
		if ct == nil {
			continue
		}
		wg.Go(func() {
			vnIds := ct.Subpolicies[0].
				Attributes.(*apstra.ConnectivityTemplatePrimitiveAttributesAttachMultipleVlan).
				TaggedVnNodeIds
			vns, err := core.GetVirtualNetworks(l.ctx, l.clients[idx], vnIds)
			if err != nil {
				return
			}
			mut.Lock()
			defer mut.Unlock()
			for _, vn := range vns {
				vnis[*vn.Data.VnId] = struct{}{}
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	var selVni uint32
	vniOptions := make([]huh.Option[uint32], 0, len(vnis))
	sortedVnis := make([]uint32, 0, len(vnis))
	for vni := range vnis {
		sortedVnis = append(sortedVnis, vni)
	}
	slices.Sort(sortedVnis)
	for _, vni := range sortedVnis {
		vniOptions = append(vniOptions, huh.NewOption(fmt.Sprintf("%d", vni), vni))
	}
	err = huh.NewSelect[uint32]().
		Title("Select VNI").
		Options(vniOptions...).
		Value(&selVni).
		Height(6).
		Run()
	if err != nil {
		return err
	}

	errors := make([]error, l.size)
	for idx, ct := range cts {
		if ct == nil {
			continue
		}
		wg.Go(func() {
			err := core.UpdateConnectivityTemplateRemoveVxlan(l.ctx, l.clients[idx], *ct.Id, []uint32{selVni})
			if err != nil {
				errors[idx] = err
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx, ct := range cts {
		if ct == nil {
			l.PrintCommandError(idx, fmt.Errorf("ct not found"))
		} else if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "removed vni %d from CT %s", selVni, ct.Label)
		}
	}

	return nil
}

// Ask the user to select a CT from all blueprints
func (l *LapstraCmd) ctSelectAll() ([]*apstra.ConnectivityTemplate, error) {
	cts := make(map[string][]*apstra.ConnectivityTemplate)
	errors := make([]error, l.size)
	wg := sync.WaitGroup{}
	mut := sync.Mutex{}
	for idx, client := range l.clients {
		wg.Go(func() {
			ct, err := client.GetAllConnectivityTemplates(l.ctx)
			if err != nil {
				errors[idx] = err
				return
			}
			mut.Lock()
			defer mut.Unlock()
			for _, c := range ct {
				if len(cts[c.Label]) == 0 {
					cts[c.Label] = make([]*apstra.ConnectivityTemplate, l.size)
				}
				cts[c.Label][idx] = &c
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()
	l.PrintCommandErrors(errors)

	nameOptions := make([]huh.Option[string], 0)
	var selectedCtName string
	for name := range cts {
		nameOptions = append(nameOptions, huh.Option[string]{
			Value: name,
			Key:   name,
		})
	}
	err := huh.NewSelect[string]().
		Title("Select Connectivity Template").
		Options(nameOptions...).
		Value(&selectedCtName).
		Height(10).
		Run()
	if err != nil {
		return nil, err
	}
	core.Logger().Infof("Selected CT: %s", selectedCtName)

	return cts[selectedCtName], nil
}

func (l *LapstraCmd) CtStaticRouteAdd() error {
	cts, err := l.ctSelectAll()
	if err != nil {
		return err
	}

	zones, err := l.AskRoutingZone("Select VRF")
	if err != nil {
		return err
	}

	_, network, err := AskIpSubnet("Enter destination network")
	if err != nil {
		return err
	}

	nextHop, err := AskIp("Enter next-hop IP")
	if err != nil {
		return err
	}

	errors := l.Errors()
	wg := sync.WaitGroup{}
	for idx, ct := range cts {
		if ct == nil {
			continue
		}
		if zones[idx] == nil {
			errors[idx] = fmt.Errorf("routing zone not found")
			continue
		}
		wg.Go(func() {
			err := core.UpdateConnectivityTemplateAddStaticRoutes(l.ctx, l.clients[idx], ct, []core.StaticRouteSpec{{
				Network:      network,
				NextHop:      nextHop,
				SecurityZone: zones[idx],
			}})
			if err != nil {
				errors[idx] = err
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx, ct := range cts {
		if ct == nil {
			continue
		}
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "added static route %s via %s to CT %s", network.String(), nextHop.String(), ct.Label)
		}
	}

	return nil
}

func (l *LapstraCmd) CtVxlanSearch() error {
	// Step 1: Ask user for VNI
	vni, err := AskVni()
	if err != nil {
		return err
	}

	// Step 2: Find VN ObjectId for the VNI in each blueprint (parallel)
	wg := sync.WaitGroup{}
	vnIds := make([]apstra.ObjectId, l.size)
	errors := l.Errors()

	for idx, client := range l.clients {
		wg.Go(func() {
			vn, err := core.GetVirtualNetworkVni(l.ctx, client, vni)
			if err != nil {
				errors[idx] = err
				return
			}
			vnIds[idx] = vn.Id
		})
	}
	spinner.New().Action(func() { wg.Wait() }).Run()

	// Step 3: Get all CTs and filter to those containing the VN (parallel)
	type ctResult struct {
		labels []string
	}
	results := make([]ctResult, l.size)

	for idx, client := range l.clients {
		if errors[idx] != nil {
			continue // Skip if VN wasn't found in this blueprint
		}
		wg.Go(func() {
			cts, err := client.GetAllConnectivityTemplates(l.ctx)
			if err != nil {
				errors[idx] = err
				return
			}

			var matchingLabels []string
			for _, ct := range cts {
				if len(ct.Subpolicies) == 0 {
					continue
				}
				if ct.Subpolicies[0].Attributes.PolicyTypeName() != apstra.CtPrimitivePolicyTypeNameAttachMultipleVlan {
					continue
				}
				taggedVns := ct.Subpolicies[0].Attributes.(*apstra.ConnectivityTemplatePrimitiveAttributesAttachMultipleVlan).TaggedVnNodeIds
				if slices.Contains(taggedVns, vnIds[idx]) {
					matchingLabels = append(matchingLabels, ct.Label)
				}
			}
			results[idx].labels = matchingLabels
		})
	}
	spinner.New().Action(func() { wg.Wait() }).Run()

	// Step 4: Report results
	for idx := range l.clients {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else if len(results[idx].labels) == 0 {
			l.PrintCommandResult(idx, "VNI %d not found in any Connectivity Template", vni)
		} else {
			l.PrintCommandResult(idx, "VNI %d found in: %s", vni, strings.Join(results[idx].labels, ", "))
		}
	}

	return nil
}
