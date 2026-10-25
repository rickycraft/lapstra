package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/Juniper/apstra-go-sdk/datacenter"
	"github.com/charmbracelet/huh/spinner"
	"rickycraft/lapstra/cmd"
	"rickycraft/lapstra/core"
)

var vlanCreateCSVHeader = []string{"vid", "name", "description", "ip", "routing-zone"}

type vlanCreateRow struct {
	number      int
	vid         uint16
	name        string
	description string
	ip          net.IP
	network     *net.IPNet
	routingZone string
	l3          bool
}

func RunVlanCreate(l *cmd.LapstraCmd) {
	path, err := vlanCreateCSVPath()
	if err != nil {
		core.Logger().Error(err)
		os.Exit(1)
	}

	rows, err := readVlanCreateCSV(path)
	if err != nil {
		core.Logger().Error(err)
		os.Exit(1)
	}

	if err := vlanCreate(l, rows); err != nil {
		core.Logger().Error(err)
		os.Exit(1)
	}
}

func vlanCreateCSVPath() (string, error) {
	if len(os.Args) == 0 {
		return "", fmt.Errorf("missing CSV path")
	}
	path := os.Args[len(os.Args)-1]
	if path == "VlanCreate" || strings.HasPrefix(path, "-") {
		return "", fmt.Errorf("usage: lapstra -a <api> -u <user> -o VlanCreate vlans.csv")
	}
	return path, nil
}

func readVlanCreateCSV(path string) ([]vlanCreateRow, error) {
	records, err := core.ReadCSVFile(path)
	if err != nil {
		return nil, err
	}

	records, firstLine := core.SkipOptionalCSVHeader(records, vlanCreateCSVHeader)
	rows := make([]vlanCreateRow, 0, len(records))
	for idx, record := range records {
		row, err := parseVlanCreateRow(firstLine+idx, record)
		if err != nil {
			return nil, err
		}
		rows = append(rows, *row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV %q has no data rows", path)
	}

	return rows, nil
}

func parseVlanCreateRow(rowNumber int, record []string) (*vlanCreateRow, error) {
	if len(record) != 3 && len(record) != 5 {
		return nil, fmt.Errorf("CSV row %d must have 3 to 5 fields", rowNumber)
	}

	for idx, name := range []string{"vid", "name", "description"} {
		if record[idx] == "" {
			return nil, fmt.Errorf("CSV row %d field %q is empty", rowNumber, name)
		}
	}

	vid, err := strconv.ParseUint(record[0], 10, 16)
	if err != nil || vid < 1 || vid > 4094 {
		return nil, fmt.Errorf("CSV row %d field %q must be a VLAN ID from 1 to 4094", rowNumber, "vid")
	}

	row := &vlanCreateRow{
		number:      rowNumber,
		vid:         uint16(vid),
		name:        record[1],
		description: record[2],
	}

	// Create L2: only 3 fields set or the other 2 are empty
	if len(record) == 3 || (record[3] == "" && record[4] == "") {
		return row, nil
	}
	// Not consistent
	if record[3] == "" {
		return nil, fmt.Errorf("CSV row %d field %q is required when routing-zone is set", rowNumber, "ip")
	}
	if record[4] == "" {
		return nil, fmt.Errorf("CSV row %d field %q is required when ip is set", rowNumber, "routing-zone")
	}

	ip, network, err := net.ParseCIDR(record[3])
	if err != nil {
		return nil, fmt.Errorf("CSV row %d field %q must be in CIDR format", rowNumber, "ip")
	}
	row.ip = ip
	row.network = network
	row.routingZone = record[4]
	row.l3 = true

	return row, nil
}

func vlanCreate(l *cmd.LapstraCmd, rows []vlanCreateRow) error {
	ctx, cancel := l.Context()
	defer cancel()

	zoneMaps, err := validateVlanCreateRoutingZones(ctx, l, rows)
	if err != nil {
		return err
	}

	for _, row := range rows {
		errors := l.Errors()
		wg := sync.WaitGroup{}
		for idx, client := range l.Clients() {
			wg.Go(func() {
				vni, err := l.MapVidVni(idx, row.vid)
				if err != nil {
					errors[idx] = err
					return
				}

				var vn *apstra.ObjectId
				if row.l3 {
					zone := zoneMaps[idx][row.routingZone]
					vn, err = core.CreateVnL3WithBinding(ctx, client, core.CreateVnL3{
						Name:        row.name,
						Vid:         row.vid,
						Vni:         vni,
						Zone:        apstra.ObjectId(*zone.ID()),
						Net:         row.network,
						Ip:          row.ip,
						Description: row.description,
					})
				} else {
					vn, err = core.CreateVnL2WithBinding(ctx, client, row.name, vni, row.vid, row.description)
				}
				if err != nil {
					errors[idx] = err
					return
				}

				if row.l3 {
					l.PrintCommandResult(idx, "row %d created %s vlan %d vni %d ip %s/%d vrf %s id %s", row.number, row.name, row.vid, vni, row.ip, cmd.NetowrkCIDR(row.network), row.routingZone, vn)
				} else {
					l.PrintCommandResult(idx, "row %d created %s vlan %d vni %d id %s", row.number, row.name, row.vid, vni, vn)
				}
			})
		}

		spinner.New().Action(func() { wg.Wait() }).Run()
		l.PrintCommandErrors(errors)
		for _, err := range errors {
			if err != nil {
				return fmt.Errorf("failed to create CSV row %d", row.number)
			}
		}
	}

	return nil
}

func validateVlanCreateRoutingZones(ctx context.Context, l *cmd.LapstraCmd, rows []vlanCreateRow) ([]map[string]*datacenter.SecurityZone, error) {
	zoneMaps := make([]map[string]*datacenter.SecurityZone, l.Size())
	errors := l.Errors()
	wg := sync.WaitGroup{}
	for idx, client := range l.Clients() {
		wg.Go(func() {
			zoneMap, err := core.GetSecurityZoneLabelMap(ctx, client)
			if err != nil {
				errors[idx] = err
				return
			}
			for _, row := range rows {
				if !row.l3 {
					continue
				}
				zone := zoneMap[row.routingZone]
				if zone == nil {
					errors[idx] = fmt.Errorf("routing zone %q not found", row.routingZone)
					return
				}
			}
			zoneMaps[idx] = zoneMap
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	l.PrintCommandErrors(errors)
	for _, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("failed to validate routing zones")
		}
	}

	return zoneMaps, nil
}
