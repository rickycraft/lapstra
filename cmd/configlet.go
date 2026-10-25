package cmd

import (
	"fmt"
	"sort"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/charmbracelet/huh"
	"rickycraft/lapstra/core"
)

func (l *LapstraCmd) ConfigletReimport() error {
	configlets := make([][]apstra.TwoStageL3ClosConfiglet, len(l.clients))
	labels := make(map[string]struct{})
	for idx, client := range l.clients {
		ctx, cancel := l.Context()
		items, err := client.GetAllConfiglets(ctx)
		cancel()
		if err != nil {
			l.PrintCommandError(idx, fmt.Errorf("list configlets: %w", err))
			continue
		}
		for _, item := range items {
			if item.Data == nil {
				l.PrintCommandError(idx, fmt.Errorf("configlet %q has no data", item.Id))
				continue
			}
			configlets[idx] = append(configlets[idx], item)
			labels[item.Data.Label] = struct{}{}
		}
	}
	if len(labels) == 0 {
		return fmt.Errorf("no blueprint configlets available")
	}
	names := make([]string, 0, len(labels))
	for label := range labels {
		names = append(names, label)
	}
	sort.Strings(names)
	options := make([]huh.Option[string], 0, len(names))
	for _, label := range names {
		options = append(options, huh.NewOption(label, label))
	}
	var selected string
	if err := huh.NewSelect[string]().Title("Select configlet label to re-import").
		Options(options...).Value(&selected).Height(10).Run(); err != nil {
		return err
	}
	var description []string
	for idx, items := range configlets {
		for _, item := range items {
			if item.Data.Label == selected {
				description = append(description, fmt.Sprintf("%s: label=%q, condition=%q", l.labels[idx], item.Data.Label, item.Data.Condition))
			}
		}
	}
	confirm, err := AskConfirm("Re-import matching configlets?", description)
	if err != nil {
		return err
	}
	if !confirm {
		return nil
	}
	for idx, items := range configlets {
		for _, item := range items {
			if item.Data.Label != selected {
				continue
			}
			ctx, cancel := l.Context()
			err := core.ReimportConfiglet(ctx, l.clients[idx], item, l.BlueprintLogger(idx))
			cancel()
			if err != nil {
				l.PrintCommandError(idx, err)
			} else {
				l.PrintCommandResult(idx, "configlet %q re-imported", selected)
			}
		}
	}
	return nil
}
