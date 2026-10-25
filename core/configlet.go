package core

import (
	"context"
	"fmt"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/charmbracelet/log"
)

func ReimportConfiglet(ctx context.Context, client *apstra.TwoStageL3ClosClient, configlet apstra.TwoStageL3ClosConfiglet, logger *log.Logger) error {
	if configlet.Data == nil || configlet.Data.Data == nil {
		return fmt.Errorf("configlet %q has no embedded catalog data", configlet.Id)
	}
	logger.Infof("existing configlet %q found", configlet.Data.Label)
	catalog, err := client.Client().GetConfigletByName(ctx, configlet.Data.Data.DisplayName)
	if err != nil {
		return fmt.Errorf("resolve catalog configlet %q: %w", configlet.Data.Data.DisplayName, err)
	}
	if catalog == nil || catalog.Data == nil {
		return fmt.Errorf("catalog configlet %q has no data", configlet.Data.Data.DisplayName)
	}
	// The SDK substitutes DisplayName for an empty label during creation.
	if configlet.Data.Label == "" {
		return fmt.Errorf("cannot preserve an empty blueprint configlet label")
	}
	replacement := apstra.TwoStageL3ClosConfigletData{
		Label:     configlet.Data.Label,
		Condition: configlet.Data.Condition,
		Data: &apstra.ConfigletData{
			DisplayName: catalog.Data.DisplayName,
			Generators:  catalog.Data.Generators,
		},
	}
	if err := client.DeleteConfiglet(ctx, configlet.Id); err != nil {
		return fmt.Errorf("delete configlet %q: %w", configlet.Data.Label, err)
	}
	logger.Infof("old configlet %q deleted", configlet.Data.Label)
	if _, err := client.CreateConfiglet(ctx, &replacement); err != nil {
		return fmt.Errorf("recreate configlet %q failed; configlet remains absent: %w", configlet.Data.Label, err)
	}
	logger.Infof("new configlet %q created", configlet.Data.Label)
	return nil
}
