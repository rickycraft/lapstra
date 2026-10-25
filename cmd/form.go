package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/IBM/fp-go/array"
	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"

	"rickycraft/lapstra/core"
)

type CommandFunc func() error

func AskBlueprint(blueprints []apstra.BlueprintStatus) ([]apstra.ObjectId, error) {

	blueprintOpts := make([]huh.Option[apstra.ObjectId], 0)
	for _, blueprint := range blueprints {
		blueprintOpts = append(blueprintOpts, huh.Option[apstra.ObjectId]{
			Value: blueprint.Id,
			Key:   blueprint.Label,
		})
	}

	var blueprintIds []apstra.ObjectId
	err := huh.NewMultiSelect[apstra.ObjectId]().
		Title("Select Blueprints").
		Options(blueprintOpts...).
		Value(&blueprintIds).
		Run()
	if err != nil {
		return nil, err
	}

	return blueprintIds, nil
}

func SelectBlueprint(apiEndpoint string, username string, password string, interactive bool) (*LapstraCmd, error) {
	client, blueprints, err := connectBlueprints(apiEndpoint, username, password)
	if err != nil {
		return nil, err
	}

	checkRaFe := func(b apstra.BlueprintStatus) bool {
		return strings.Contains(b.Label, "RA") || strings.Contains(b.Label, "FE")
	}
	isRAFE := array.Any(checkRaFe)(blueprints)

	// Make the user choose location
	locationOpts := []huh.Option[Location]{
		{Value: DatacenterPair, Key: "ALL"},
		{Value: Custom, Key: "Custom"},
	}
	if isRAFE {
		locationOpts = append(locationOpts, huh.Option[Location]{Value: RA, Key: "RA"})
		locationOpts = append(locationOpts, huh.Option[Location]{Value: FE, Key: "FE"})
	} else {
		locationOpts = append(locationOpts, huh.Option[Location]{Value: MO, Key: "MO"})
		locationOpts = append(locationOpts, huh.Option[Location]{Value: PR, Key: "PR"})
	}
	// If location is Custom, then prompt for the blueprint name
	location := DatacenterPair // Reset default value
	if interactive {
		err = huh.NewSelect[Location]().
			Title("Select Location").
			Options(locationOpts...).
			Value(&location).
			Run()
		if err != nil {
			return nil, err
		}
	}

	// select the speecific blueprint
	l := LapstraCmd{}
	err = l.Connect(client, blueprints, location)
	if err != nil {
		return nil, err
	}

	return &l, nil
}

func connectBlueprints(apiEndpoint string, username string, password string) (*apstra.Client, []apstra.BlueprintStatus, error) {

	var (
		client     *apstra.Client
		blueprints []apstra.BlueprintStatus
		err        error
	)
	// If the user takes log to choose the blueprints it can get cancelled
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	// Connect to Apstra and get all blueprints
	err = spinner.New().Title("Connecting to Apstra").ActionWithErr(
		func(_ context.Context) error {
			client, err = core.ConnectToApstra(ctx, apiEndpoint, username, password)
			if err != nil {
				return err
			}

			blueprints, err = client.GetAllBlueprintStatus(ctx)
			if err != nil {
				return err
			}
			return nil
		}).Run()
	if err != nil {
		return nil, nil, err
	}
	return client, blueprints, nil
}

func SelectSingleBlueprint(apiEndpoint string, username string, password string) (*LapstraCmd, error) {
	client, blueprints, err := connectBlueprints(apiEndpoint, username, password)
	if err != nil {
		return nil, err
	}

	options := make([]huh.Option[int], 0, len(blueprints))
	for idx, blueprint := range blueprints {
		options = append(options, huh.NewOption(blueprint.Label, idx))
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no blueprints found")
	}

	selected := 0
	err = huh.NewSelect[int]().
		Title("Select Blueprint").
		Options(options...).
		Value(&selected).
		Height(10).
		Run()
	if err != nil {
		return nil, err
	}

	l := LapstraCmd{}
	err = l.Connect(client, []apstra.BlueprintStatus{blueprints[selected]}, DatacenterPair)
	if err != nil {
		return nil, err
	}

	return &l, nil
}

func ExitBlueprint() error {
	os.Exit(0)
	return nil
}

func SelectCmd(l *LapstraCmd) (CommandFunc, error) {

	var cmdRegistry = []CommandFunc{
		ExitBlueprint,
		// l.VlanShow,
		l.VlanSearch,
		l.VlanCreateL2,
		l.VlanCreateL3,
		l.TransformVlanL3,
		// l.VlanDelete, Risky for vlan not stretched
		l.VniDelete,
		// l.VniPoolShow,
		l.CtVlanSearch,
		l.CtMultiVlanCreate,
		// l.CtMultiVlanRemove,
		l.CtVxlanAdd,
		l.CtVxlanRemove,
		l.CtVxlanSearch,
		l.CtStaticRouteAdd,
		l.AddStaticRouteCSV,
		l.RoutingZoneCreate,
		// l.RoutingZoneDelete,
		// l.RoutingZoneShow,
		l.ConfigletReimport,
		l.CheckConfig,
		l.StartDeploy,
		l.RevertBlueprints,
	}

	commandNames := []string{
		"Exit",
		// "VlanShow",
		"VlanSearch",
		"VlanCreateL2",
		"VlanCreateL3",
		"TransformVlanL3",
		// "VlanDelete",
		"VniDelete",
		// "VniPoolShow",
		"CtVlanSearch",
		"CtMultiVlanCreate",
		// "CtMultiVlanRemove",
		"CtVxlanAdd",
		"CtVxlanRemove",
		"CtVxlanSearch",
		"CtStaticRouteAdd",
		"AddStaticRouteCSV",
		"RoutingZoneCreate",
		// "RoutingZoneDelete",
		// "RoutingZoneShow",
		"ConfigletReimport",
		"CheckConfig",
		"Deploy",
		"Revert",
	}
	opts := make([]huh.Option[int], len(commandNames))
	for i, name := range commandNames[1:] {
		opts[i] = huh.NewOption(name, i+1)
	}
	opts[len(opts)-1] = huh.NewOption(commandNames[0], 0)

	cmdIdx := 1
	form := huh.NewSelect[int]().
		Title("Select command").
		Options(opts...).
		Value(&cmdIdx)
	err := form.Run()
	if err != nil {
		return nil, err
	}

	core.Logger().Infof("%s", commandNames[cmdIdx])
	return cmdRegistry[cmdIdx], nil
}
