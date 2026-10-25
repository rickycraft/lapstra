package cmd

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"rickycraft/lapstra/core"
)

const (
	staticRouteCSVFields = 4
	staticRouteCSVHeader = "connectivity_template,security_zone,network,next_hop"
)

type staticRouteCSVRecord struct {
	CtLabel           string
	SecurityZoneLabel string
	Network           *net.IPNet
	NextHop           net.IP
}

func (l *LapstraCmd) AddStaticRouteCSV() error {
	l.Logger().Debug("scanning current directory for static route CSV files")
	files, err := core.CSVFilesInDir(".")
	if err != nil {
		return fmt.Errorf("failed to list CSV files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no .csv files found in current directory")
	}
	l.Logger().Debugf("found %d CSV files", len(files))

	var csvPath string
	opts := make([]huh.Option[string], 0, len(files))
	for _, file := range files {
		opts = append(opts, huh.NewOption(file, file))
	}
	err = huh.NewSelect[string]().
		Title("Select static route CSV").
		Description("Optional header: " + staticRouteCSVHeader).
		Options(opts...).
		Value(&csvPath).
		Height(10).
		Run()
	if err != nil {
		return fmt.Errorf("failed to select static route CSV: %w", err)
	}

	return l.ImportStaticRoutesCSV(csvPath)
}

func (l *LapstraCmd) ImportStaticRoutesCSV(csvPath string) error {
	records, err := readStaticRouteCSV(csvPath)
	if err != nil {
		return fmt.Errorf("failed to read static route CSV %q: %w", csvPath, err)
	}
	l.Logger().Debugf("loaded %d static routes from %s", len(records), csvPath)
	for _, record := range records {
		l.Logger().Debugf("loaded static route from csv: ct %q vrf %q network %s next-hop %s", record.CtLabel, record.SecurityZoneLabel, record.Network, record.NextHop)
	}
	if len(records) == 0 {
		return fmt.Errorf("no static routes found in %s", csvPath)
	}
	routeGroup := groupStaticRoute(records)

	ctx, cancel := l.Context()
	defer cancel()

	var wg sync.WaitGroup
	for idx, client := range l.Clients() {
		wg.Go(func() {
			securityZones, err := core.GetSecurityZoneLabelMap(ctx, client)
			if err != nil {
				l.PrintCommandError(idx, fmt.Errorf("security zone lookup failed: %w", err))
				return
			}
			l.BlueprintLogger(idx).Debugf("loaded %d security zones", len(securityZones))

			connectivityTemplates, err := core.GetConnectivityTemplateLabelMap(ctx, client)
			if err != nil {
				l.PrintCommandError(idx, fmt.Errorf("connectivity template lookup failed: %w", err))
				return
			}
			l.BlueprintLogger(idx).Debugf("loaded %d connectivity templates", len(connectivityTemplates))

			for ctLabel, records := range routeGroup {
				routes := make([]core.StaticRouteSpec, 0, len(records))
				for _, record := range records {
					zone := securityZones[record.SecurityZoneLabel]
					if zone == nil {
						l.PrintCommandError(idx, fmt.Errorf("static route %s via %s to CT %q: security zone %q not found", record.Network, record.NextHop, ctLabel, record.SecurityZoneLabel))
						continue
					}
					routes = append(routes, core.StaticRouteSpec{
						Network:      record.Network,
						NextHop:      record.NextHop,
						SecurityZone: zone,
					})
				}
				if len(routes) == 0 {
					l.PrintCommandError(idx, fmt.Errorf("for ct %s no routes", ctLabel))
					continue
				}

				ct := connectivityTemplates[ctLabel]
				created := false
				var leafs []core.SystemInfo
				if ct == nil {
					leafs, err = core.GetLeafs(ctx, client)
					if err != nil {
						l.PrintCommandError(idx, fmt.Errorf("connectivity template %q: leaf lookup failed: %w", ctLabel, err))
						continue
					}
					if len(leafs) != 2 {
						l.PrintCommandError(idx, fmt.Errorf("connectivity template %q: expected 2 leafs, found %d", ctLabel, len(leafs)))
						continue
					}

					l.BlueprintLogger(idx).Debugf("connectivity template %q not found; creating it", ctLabel)
					ct, err = core.CreateConnectivityTemplate(ctx, client, ctLabel)
					if err != nil {
						l.PrintCommandError(idx, fmt.Errorf("connectivity template %q: creation failed: %w", ctLabel, err))
						continue
					}
					created = true
				}

				err = core.UpdateConnectivityTemplateAddStaticRoutes(ctx, client, ct, routes)
				if err != nil {
					err = fmt.Errorf("static routes to CT %q (%d routes): update failed: %w; earlier changes may already have been applied", ctLabel, len(routes), err)
					l.PrintCommandError(idx, err)
					continue
				}

				if created {
					err = core.AssignConnectivityTemplateToLeafs(ctx, client, *ct.Id, leafs)
					if err != nil {
						l.PrintCommandError(idx, fmt.Errorf("connectivity template %q: assignment to both leafs failed: %w; template creation and route updates have already been applied", ctLabel, err))
						continue
					}
					l.BlueprintLogger(idx).Debugf("created connectivity template %q and assigned it to both leafs", ctLabel)
				}

				for _, route := range routes {
					l.PrintCommandResult(idx, "[%s] add %s via %s in %s", ct.Label, route.Network.String(), route.NextHop.String(), route.SecurityZone.Label)
				}
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	return nil
}

func groupStaticRoute(records []staticRouteCSVRecord) map[string][]staticRouteCSVRecord {
	groups := make(map[string][]staticRouteCSVRecord)
	for _, record := range records {
		groups[record.CtLabel] = append(groups[record.CtLabel], record)
	}
	return groups
}

func readStaticRouteCSV(path string) ([]staticRouteCSVRecord, error) {
	rows, err := core.ReadCSVFile(path)
	if err != nil {
		return nil, err
	}

	header := strings.Split(staticRouteCSVHeader, ",")
	rows, firstLine := core.SkipOptionalCSVHeader(rows, header)
	records := make([]staticRouteCSVRecord, 0, len(rows))
	for idx, row := range rows {
		line := firstLine + idx

		record, err := parseStaticRouteCSVRow(line, row)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CSV row: %w", err)
		}
		records = append(records, record)
	}

	return records, nil
}

func parseStaticRouteCSVRow(line int, row []string) (staticRouteCSVRecord, error) {
	if len(row) != staticRouteCSVFields {
		return staticRouteCSVRecord{}, fmt.Errorf("line %d: expected %d fields, got %d", line, staticRouteCSVFields, len(row))
	}
	for idx := range row {
		row[idx] = strings.TrimSpace(row[idx])
	}

	ctLabel := row[0]
	securityZoneLabel := row[1]
	networkString := row[2]
	nextHopString := row[3]
	if ctLabel == "" || securityZoneLabel == "" || networkString == "" || nextHopString == "" {
		return staticRouteCSVRecord{}, fmt.Errorf("line %d: invalid static route CSV row", line)
	}

	_, network, err := net.ParseCIDR(networkString)
	nextHop := net.ParseIP(nextHopString)
	if err != nil || nextHop == nil {
		return staticRouteCSVRecord{}, fmt.Errorf("line %d: invalid static route CSV row", line)
	}

	return staticRouteCSVRecord{
		CtLabel:           ctLabel,
		SecurityZoneLabel: securityZoneLabel,
		Network:           network,
		NextHop:           nextHop,
	}, nil
}
