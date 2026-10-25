package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/charmbracelet/huh"
	"rickycraft/lapstra/cmd"
	"rickycraft/lapstra/core"
)

var ctCSVHeader = []string{"ct-name", "vni"}

type ctCSVRow struct {
	number int
	name   string
	vni    uint32
}

// ctMemberships groups ordered VN IDs by template name.
type ctMemberships map[string][]apstra.ObjectId

// attachMultipleVlanAttributes returns the VLAN attributes only for a simple
// connectivity template containing exactly one AttachMultipleVlan subpolicy.
func attachMultipleVlanAttributes(ct *apstra.ConnectivityTemplate) (*apstra.ConnectivityTemplatePrimitiveAttributesAttachMultipleVlan, bool) {
	if ct == nil || len(ct.Subpolicies) != 1 || ct.Subpolicies[0] == nil {
		return nil, false
	}
	attributes, ok := ct.Subpolicies[0].Attributes.(*apstra.ConnectivityTemplatePrimitiveAttributesAttachMultipleVlan)
	return attributes, ok && attributes != nil
}

// replaceCtVlans replaces the existing template subpolicies
// with one AttachMultipleVlan primitive containing the requested memberships.
func replaceCtVlans(ctx context.Context, client *apstra.TwoStageL3ClosClient, ct *apstra.ConnectivityTemplate, vnIDs []apstra.ObjectId) error {
	core.Logger().Debugf("replacing connectivity template %q with AttachMultipleVlan and %d virtual networks", ct.Label, len(vnIDs))
	attributes := apstra.ConnectivityTemplatePrimitiveAttributesAttachMultipleVlan{
		Label:           ct.Label + "-Vlan",
		TaggedVnNodeIds: vnIDs,
	}
	primitive := apstra.ConnectivityTemplatePrimitive{
		Label:      "VlanMultiple-Primitive",
		Attributes: &attributes,
	}
	ct.Subpolicies = []*apstra.ConnectivityTemplatePrimitive{&primitive}
	if err := client.UpdateConnectivityTemplate(ctx, ct); err != nil {
		return fmt.Errorf("error updating connectivity template: %w", err)
	}
	return nil
}

// RunCtExport runs the connectivity template CSV export and terminates with a
// non-zero status when validation, API access, or file writing fails.
func RunCtExport(l *cmd.LapstraCmd) {
	if err := exportCtCSV(l); err != nil {
		core.Logger().Error(err)
		os.Exit(1)
	}
}

// exportCtCSV collects supported L2 connectivity template memberships from one
// blueprint, validates every VN reference, and writes them to the export CSV.
func exportCtCSV(l *cmd.LapstraCmd) error {
	if l.Size() != 1 {
		return fmt.Errorf("CtExport requires exactly one blueprint")
	}
	l.BlueprintLogger(0).Debug("starting connectivity template CSV export")

	ctx, cancel := l.Context()
	defer cancel()
	client := l.Clients()[0]

	cts, err := client.GetAllConnectivityTemplates(ctx)
	if err != nil {
		return fmt.Errorf("get connectivity templates: %w", err)
	}
	vns, err := client.GetAllVirtualNetworks(ctx)
	if err != nil {
		return fmt.Errorf("get virtual networks: %w", err)
	}
	l.BlueprintLogger(0).Debugf("loaded %d connectivity templates and %d virtual networks", len(cts), len(vns))

	vniByID := make(map[apstra.ObjectId]uint32, len(vns))
	for id, vn := range vns {
		if vn.Data != nil && vn.Data.VnId != nil {
			vniByID[id] = *vn.Data.VnId
		}
	}
	l.BlueprintLogger(0).Debugf("cached VNI values for %d virtual networks", len(vniByID))

	records := make([][]string, 0)
	templateCount := 0
	for idx := range cts {
		ct := &cts[idx]
		attributes, ok := attachMultipleVlanAttributes(ct)
		if !ok {
			l.BlueprintLogger(0).Debugf("skipping connectivity template %q: not a simple AttachMultipleVlan template", ct.Label)
			continue
		}
		if len(attributes.TaggedVnNodeIds) == 0 {
			l.BlueprintLogger(0).Debugf("skipping connectivity template %q: no tagged memberships", ct.Label)
			continue
		}
		if attributes.UntaggedVnNodeId != nil {
			return fmt.Errorf("connectivity template %q has both tagged and untagged VLAN memberships; CSV export supports tagged memberships only", ct.Label)
		}

		templateRecords := make([][]string, 0, len(attributes.TaggedVnNodeIds))
		for _, vnID := range attributes.TaggedVnNodeIds {
			vni, ok := vniByID[vnID]
			if !ok {
				return fmt.Errorf("connectivity template %q references unresolved virtual network %q", ct.Label, vnID)
			}
			templateRecords = append(templateRecords, []string{ct.Label, strconv.FormatUint(uint64(vni), 10)})
		}
		records = append(records, templateRecords...)
		templateCount++
		l.BlueprintLogger(0).Debugf("collected %d memberships from connectivity template %q", len(templateRecords), ct.Label)
	}

	filename := l.Labels()[0] + "-l2-ct.csv"
	l.BlueprintLogger(0).Debugf("writing %d rows to %s", len(records), filename)
	output := make([][]string, 0, len(records)+1)
	output = append(output, ctCSVHeader)
	output = append(output, records...)
	if err := core.WriteCSVFile(filename, output); err != nil {
		return fmt.Errorf("export connectivity templates: %w", err)
	}

	l.PrintCommandResult(0, "exported %d rows from %d templates to %s", len(records), templateCount, filename)
	return nil
}

// RunCtImport runs the interactive connectivity template CSV import and
// terminates with a non-zero status if validation or an API mutation fails.
func RunCtImport(l *cmd.LapstraCmd) {
	if err := importCtCSV(l); err != nil {
		core.Logger().Error(err)
		os.Exit(1)
	}
}

// importCtCSV selects a CSV, validates all rows, asks for confirmation once,
// then creates or replaces templates in CSV order.
func importCtCSV(l *cmd.LapstraCmd) error {
	if l.Size() != 1 {
		return fmt.Errorf("CtImport requires exactly one blueprint")
	}
	l.BlueprintLogger(0).Debug("starting connectivity template CSV import")

	files, err := core.CSVFilesInDir(".")
	if err != nil {
		return fmt.Errorf("list CSV files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no .csv files found in current directory")
	}
	l.BlueprintLogger(0).Debugf("found %d CSV files in current directory", len(files))

	options := make([]huh.Option[string], 0, len(files))
	for _, filename := range files {
		options = append(options, huh.NewOption(filename, filename))
	}
	var filename string
	err = huh.NewSelect[string]().
		Title("Select connectivity template CSV").
		Description("Optional header: ct-name,vni").
		Options(options...).
		Value(&filename).
		Height(10).
		Run()
	if err != nil {
		return fmt.Errorf("select CSV file: %w", err)
	}
	l.BlueprintLogger(0).Debugf("selected connectivity template CSV %s", filename)

	rows, err := readCtCSV(filename)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		l.PrintCommandResult(0, "no rows to import from %s", filename)
		return nil
	}
	l.BlueprintLogger(0).Debugf("loaded %d rows from %s", len(rows), filename)

	ctx, cancel := l.Context()
	defer cancel()
	client := l.Clients()[0]

	vns, err := client.GetAllVirtualNetworks(ctx)
	if err != nil {
		return fmt.Errorf("get virtual networks: %w", err)
	}
	l.BlueprintLogger(0).Debugf("loaded %d virtual networks", len(vns))
	vnByVNI := make(map[uint32]apstra.VirtualNetwork, len(vns))
	for _, vn := range vns {
		if vn.Data != nil && vn.Data.VnId != nil {
			vnByVNI[*vn.Data.VnId] = vn
		}
	}
	l.BlueprintLogger(0).Debugf("cached %d virtual networks by VNI", len(vnByVNI))

	cts, err := client.GetAllConnectivityTemplates(ctx)
	if err != nil {
		return fmt.Errorf("get connectivity templates: %w", err)
	}
	l.BlueprintLogger(0).Debugf("loaded %d connectivity templates", len(cts))
	ctByName := make(map[string]*apstra.ConnectivityTemplate, len(cts))
	for idx := range cts {
		if _, exists := ctByName[cts[idx].Label]; !exists {
			ctByName[cts[idx].Label] = &cts[idx]
		}
	}

	ctNames := make([]string, 0)
	vnIDsByCT := make(ctMemberships)
	preview := make([]string, 0, len(rows))
	for _, row := range rows {
		vn, ok := vnByVNI[row.vni]
		if !ok {
			l.BlueprintLogger(0).Warnf("CSV row %d: VNI %d not found in blueprint; skipping row", row.number, row.vni)
			continue
		}
		preview = append(preview, fmt.Sprintf("%s - %s", row.name, vn.Data.Label))

		if _, exists := vnIDsByCT[row.name]; !exists {
			ctNames = append(ctNames, row.name)
		}
		vnIDsByCT[row.name] = append(vnIDsByCT[row.name], vn.Id)
	}
	l.BlueprintLogger(0).Debugf("grouped %d CSV rows into %d connectivity templates", len(preview), len(ctNames))
	if len(ctNames) == 0 {
		l.PrintCommandResult(0, "no rows to import from %s", filename)
		return nil
	}

	l.BlueprintLogger(0).Debug("validated virtual network references for import")

	for _, line := range preview {
		fmt.Println(line)
	}
	confirm, err := cmd.AskConfirm(
		"Import connectivity templates?",
		[]string{"Existing connectivity template memberships will be replaced."},
	)
	if err != nil {
		return fmt.Errorf("confirm import: %w", err)
	}
	if !confirm {
		l.BlueprintLogger(0).Debug("connectivity template CSV import cancelled by user")
		l.PrintCommandResult(0, "import cancelled; no changes applied")
		return nil
	}

	// First create every missing template with an empty membership list.
	createdCTIDs := make(map[string]apstra.ObjectId)
	for _, ctName := range ctNames {
		if _, exists := ctByName[ctName]; exists {
			l.BlueprintLogger(0).Debugf("connectivity template %q already exists", ctName)
			continue
		}
		l.BlueprintLogger(0).Debugf("creating empty connectivity template %q", ctName)
		ctID, err := core.CreateConnectivityTemplateVlans(ctx, client, ctName, nil)
		if err != nil {
			return fmt.Errorf("create connectivity template %q: %w; earlier changes may already have been applied", ctName, err)
		}
		createdCTIDs[ctName] = *ctID
	}
	l.BlueprintLogger(0).Debugf("created %d missing connectivity templates", len(createdCTIDs))

	// Then replace the complete membership of every template from the CSV.
	for _, ctName := range ctNames {
		if ctID, created := createdCTIDs[ctName]; created {
			l.BlueprintLogger(0).Debugf("loading newly created connectivity template %q", ctName)
			ct, err := client.GetConnectivityTemplate(ctx, ctID)
			if err != nil {
				return fmt.Errorf("get newly created connectivity template %q: %w; earlier changes may already have been applied", ctName, err)
			}
			ctByName[ctName] = ct
		}
		vnIDs := vnIDsByCT[ctName]
		l.BlueprintLogger(0).Debugf("updating connectivity template %q with %d memberships", ctName, len(vnIDs))
		if err := replaceCtVlans(ctx, client, ctByName[ctName], vnIDs); err != nil {
			return fmt.Errorf("update connectivity template %q: %w; earlier changes may already have been applied", ctName, err)
		}
		l.PrintCommandResult(0, "imported connectivity template %s with %d memberships", ctName, len(vnIDs))
	}
	return nil
}

// readCtCSV reads the strict ct-name,vni format while preserving template names
// verbatim and trimming only the numeric VNI field before parsing it.
func readCtCSV(path string) ([]ctCSVRow, error) {
	records, err := core.ReadCSVFile(path)
	if err != nil {
		return nil, err
	}

	records, firstLine := core.SkipOptionalCSVHeader(records, ctCSVHeader)
	rows := make([]ctCSVRow, 0, len(records))
	for idx, record := range records {
		rowNumber := firstLine + idx
		if len(record) != 2 {
			return nil, fmt.Errorf("CSV row %d must contain exactly two fields", rowNumber)
		}
		vniText := strings.TrimSpace(record[1])
		vni, err := strconv.ParseUint(vniText, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("CSV row %d has invalid VNI %q: %w", rowNumber, record[1], err)
		}
		rows = append(rows, ctCSVRow{number: rowNumber, name: record[0], vni: uint32(vni)})
	}
	return rows, nil
}
