package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/charmbracelet/huh/spinner"
	"rickycraft/lapstra/cmd"
	"rickycraft/lapstra/core"
)

type vniMergeRow struct {
	vlan     uint16
	site     cmd.Location
	removeMx bool
}

var vniMergeCSVHeader = []string{"vlan", "site", "remove_mx"}

// RunMergeVlan loads VNI merge rows from the CSV argument and applies them.
func RunMergeVlan(l *cmd.LapstraCmd) {

	argLen := len(os.Args)
	rows, err := readVniMergeCSV(os.Args[argLen-1])
	if err != nil {
		core.Logger().Error(err)
		os.Exit(1)
	}

	err = mergeVlan(l, rows)
	if err != nil {
		os.Exit(1)
	}
}

// readVniMergeCSV reads the VNI merge CSV file into validated row values.
func readVniMergeCSV(path string) ([]vniMergeRow, error) {
	records, err := core.ReadCSVFile(path)
	if err != nil {
		return nil, err
	}

	records, firstLine := core.SkipOptionalCSVHeader(records, vniMergeCSVHeader)
	rows := make([]vniMergeRow, 0, len(records))
	for idx, record := range records {
		line := firstLine + idx

		row, err := parseVniMergeRow(record)
		if err != nil {
			return nil, fmt.Errorf("invalid CSV row %d: %w", line, err)
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// parseVniMergeRow validates and converts one VNI merge CSV record.
func parseVniMergeRow(record []string) (vniMergeRow, error) {
	if len(record) != 3 {
		return vniMergeRow{}, fmt.Errorf("expected columns vlan,site,remove_mx")
	}

	vlanID, err := strconv.ParseUint(record[0], 10, 16)
	if err != nil {
		return vniMergeRow{}, fmt.Errorf("invalid VLAN ID '%s': %w", record[0], err)
	}

	site := cmd.DcToLocation(record[1])
	switch site {
	case cmd.RA, cmd.FE, cmd.MO, cmd.PR:
	default:
		return vniMergeRow{}, fmt.Errorf("invalid site '%s' should be one of: RA FE MO PR", record[1])
	}

	removeMx, err := strconv.ParseBool(record[2])
	if err != nil {
		return vniMergeRow{}, fmt.Errorf("invalid remove_mx '%s': %w", record[2], err)
	}

	return vniMergeRow{
		vlan:     uint16(vlanID),
		site:     site,
		removeMx: removeMx,
	}, nil
}

// mergeVlan applies all row VNI updates and then removes flagged VLANs from CTs.
func mergeVlan(l *cmd.LapstraCmd, rows []vniMergeRow) error {

	removeMxVlans := make([]uint16, 0)

	// Change all the VNIs
	for _, row := range rows {
		l.Logger().Debugf("processing vlan %d site %s remove_mx %t", row.vlan, row.site, row.removeMx)
		if _, err := mergeVni(l, row); err != nil {
			return err
		}
		if row.removeMx {
			removeMxVlans = append(removeMxVlans, row.vlan)
		}
	}

	if len(removeMxVlans) == 0 {
		return nil
	}

	// Update the CT to the MX to remove the vlans
	return removeVlanTemplate(l, removeMxVlans)
}

// mergeVni copies the master site's VNI for one VLAN to the other blueprints.
func mergeVni(l *cmd.LapstraCmd, row vniMergeRow) ([]*apstra.VirtualNetwork, error) {

	vns := make([]*apstra.VirtualNetwork, l.Size())
	errors := l.Errors()
	wg := sync.WaitGroup{}
	masterBlueprint := -1
	ctx, cancel := l.Context()
	defer cancel()
	for idx, client := range l.Clients() {
		wg.Go(func() {
			vn, err := core.GetVirtualNetworkVid(ctx, client, row.vlan)
			if err != nil {
				errors[idx] = err
				return
			}

			vns[idx] = vn
			l.BlueprintLogger(idx).Debugf("found virtual network %s with vni %d for vlan %d", vn.Data.Label, *vn.Data.VnId, row.vlan)
		})
	}
	spinner.New().Action(func() { wg.Wait() }).Run()

	l.PrintCommandErrors(errors)
	for _, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("not all blueprint have the virtual network for vlan %d: %w", row.vlan, err)
		}
	}

	// Check for master vni/blueprint
	for idx := range l.Clients() {
		if l.BpLocation(idx) == row.site {
			masterBlueprint = idx
			break
		}
	}
	if masterBlueprint == -1 {
		core.Logger().Errorf("No master blueprint found for site %s", row.site)
		return nil, fmt.Errorf("no master blueprint found for site %s", row.site)
	}
	masterVni := vns[masterBlueprint].Data.VnId
	l.BlueprintLogger(masterBlueprint).Debugf("selected master VNI %d for vlan %d from site %s", *masterVni, row.vlan, row.site)

	// Update the VNIS that are not the same as the master
	vniChanged := false
	for idx, client := range l.Clients() {
		if vns[idx] == nil {
			core.Logger().Fatalf("virtual network not found for vlan %d (should have been caught earlier)", row.vlan)
		}
		if *vns[idx].Data.VnId == *masterVni {
			l.PrintCommandResult(idx, "VNI same as master for vlan %d", row.vlan)
			continue
		}

		vniChanged = true
		wg.Go(func() {
			vn := vns[idx]
			oldVni := vn.Data.VnId
			vn.Data.VnId = masterVni
			vn.Data.Tags = nil
			err := client.UpdateVirtualNetwork(ctx, vn.Id, vn.Data)
			if err != nil {
				errors[idx] = err
			} else {
				l.PrintCommandResult(idx, "VNI updated for vlan %d - [%d -> %d]", row.vlan, *oldVni, *masterVni)
			}
		})
	}
	spinner.New().Action(func() { wg.Wait() }).Run()

	l.PrintCommandErrors(errors)
	for _, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("failed to update all vnis for vlan %d: %w", row.vlan, err)
		}
	}
	if !vniChanged {
		l.Logger().Warnf("no VNI changed for vlan %d", row.vlan)
	}

	return vns, nil
}

// removeVlanTemplate removes the selected VLANs from each L2 dc- connectivity template.
func removeVlanTemplate(l *cmd.LapstraCmd, vlans []uint16) error {

	l.Logger().Debugf("removing vlans %v from L2 dc- connectivity templates", vlans)

	errors := make([]error, l.Size())
	wg := sync.WaitGroup{}
	ctx, cancel := l.Context()
	defer cancel()
	for idx, client := range l.Clients() {
		wg.Go(func() {
			ctall, err := client.GetAllConnectivityTemplates(ctx)
			if err != nil {
				errors[idx] = err
				return
			}

			for _, c := range ctall {
				if strings.Contains(c.Label, "L2 dc-") {

					err = core.UpdateConnectivityTemplateRemoveVid(ctx, client, *c.Id, vlans)
					if err != nil {
						errors[idx] = err
					} else {
						l.PrintCommandResult(idx, "connectivity template %s updated", c.Label)
					}
					break
				}
			}

		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()
	l.PrintCommandErrors(errors)
	for _, err := range errors {
		if err != nil {
			return fmt.Errorf("failed to remove vlans from CT")
		}
	}
	return nil
}
