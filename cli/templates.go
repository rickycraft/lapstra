package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/charmbracelet/huh"
	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"
	"rickycraft/lapstra/cmd"
	"rickycraft/lapstra/core"
)

// TemplatesDir is resolved relative to the process working directory, with an installed fallback.
var TemplatesDir = "./templates"

// RunTemplates reports failures and treats canceled selectors as a normal exit.
func RunTemplates(l *cmd.LapstraCmd) {
	if err := runTemplates(l); err != nil && !errors.Is(err, huh.ErrUserAborted) {
		fmt.Fprintf(os.Stderr, "Templates: %v\n", err)
		os.Exit(1)
	}
}

// leafName provides a display name even when the hostname is unavailable.
func leafName(node apstra.SystemNodeInfo) string {
	if node.Hostname != "" {
		return node.Hostname
	}
	if node.Label != "" {
		return node.Label
	}
	return string(node.Id)
}

// runTemplates coordinates selection, context retrieval, and configuration output.
func runTemplates(l *cmd.LapstraCmd) error {
	// Work with the single blueprint selected by the CLI.
	if l.Size() != 1 {
		return fmt.Errorf("requires exactly one blueprint")
	}
	ctx, cancel := l.Context()
	defer cancel()
	client := l.Clients()[0]

	// Select the leaf once; every template in this session uses the same device.
	leaf, err := selectTemplateLeaf(ctx, client)
	if err != nil {
		return err
	}

	// Fetch the selected leaf's context before opening the template menu.
	l.BlueprintLogger(0).Infof("downloading config context for leaf %s (%s)", leafName(*leaf), leaf.Id)
	raw, err := core.GetConfigContext(ctx, client, leaf.Id)
	if err != nil {
		return fmt.Errorf("leaf %s (%s): %w", leafName(*leaf), leaf.Id, err)
	}

	// Return to the template menu after each render, reusing the same context.
	for {
		filename, err := selectTemplate()
		if err != nil {
			return err
		}
		if filename == "" {
			return nil
		}

		// Render completely before writing, so a template error produces no partial config.
		result, err := renderTemplate(filename, raw)
		if err != nil {
			return fmt.Errorf("leaf %s (%s): %w", leafName(*leaf), leaf.Id, err)
		}
		// Delimit each successful render so consecutive outputs are easy to identify.
		if _, err := fmt.Fprintf(os.Stdout, "#### %s %s ####\n\n%s\n\n###\n", leafName(*leaf), filepath.Base(filename), result); err != nil {
			return fmt.Errorf("print configuration: %w", err)
		}
	}
}

// selectTemplateLeaf lists all leaves without requiring a redundancy group or pair.
func selectTemplateLeaf(ctx context.Context, client *apstra.TwoStageL3ClosClient) (*apstra.SystemNodeInfo, error) {
	// Discover system nodes and retain only leaves.
	core.Logger().Debugf("searching for leaf node IDs in blueprint %s", client.Id())
	nodes, err := client.GetAllSystemNodeInfos(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover leaves in blueprint %s: %w", client.Id(), err)
	}
	var leaves []apstra.SystemNodeInfo
	for _, node := range nodes {
		if node.Role == apstra.SystemRoleLeaf {
			leaves = append(leaves, node)
		}
	}

	// Sort by display name, using IDs to make duplicate names deterministic.
	sort.Slice(leaves, func(i, j int) bool {
		a, b := leafName(leaves[i]), leafName(leaves[j])
		if a == b {
			return leaves[i].Id < leaves[j].Id
		}
		return a < b
	})
	if len(leaves) == 0 {
		return nil, fmt.Errorf("no leaves in blueprint %s", client.Id())
	}

	// Show the ID alongside each name to distinguish otherwise identical options.
	options := make([]huh.Option[int], len(leaves))
	for i, leaf := range leaves {
		options[i] = huh.NewOption(fmt.Sprintf("%s (%s)", leafName(leaf), leaf.Id), i)
	}
	var selected int
	if err := huh.NewSelect[int]().Title("Select Leaf").Options(options...).Value(&selected).Height(10).Run(); err != nil {
		return nil, fmt.Errorf("select leaf: %w", err)
	}
	return &leaves[selected], nil
}

// selectTemplate returns the chosen full path, or an empty path for Exit.
func selectTemplate() (string, error) {
	files, err := discoverTemplates()
	if err != nil {
		return "", err
	}
	templateOptions := make([]huh.Option[string], 0, len(files)+1)
	for _, file := range files {
		templateOptions = append(templateOptions, huh.NewOption(filepath.Base(file), file))
	}
	templateOptions = append(templateOptions, huh.NewOption("Exit", ""))
	filename := files[0]
	if err := huh.NewSelect[string]().Title("Select Template").Options(templateOptions...).Value(&filename).Height(10).Run(); err != nil {
		return "", fmt.Errorf("select template: %w", err)
	}
	return filename, nil
}

// discoverTemplates finds regular .j2 files locally or in the installed directory.
func discoverTemplates() ([]string, error) {
	// Resolve relative paths from the working directory, not the executable location.
	dir, err := filepath.Abs(TemplatesDir)
	if err != nil {
		return nil, fmt.Errorf("resolve templates directory %q: %w", TemplatesDir, err)
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		const installedDir = "/usr/local/src/lapstra"
		entries, err = os.ReadDir(installedDir)
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("templates directories %q and %q do not exist", dir, installedDir)
		}
		dir = installedDir
	}
	if err != nil {
		return nil, fmt.Errorf("read templates directory %q: %w", dir, err)
	}

	// ReadDir is already sorted; skip directories, symlinks, and other extensions.
	var files []string
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".j2" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect template %q: %w", filepath.Join(dir, entry.Name()), err)
		}
		if info.Mode().IsRegular() {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no regular .j2 templates in %q", dir)
	}
	return files, nil
}

// renderTemplate exposes context fields at the top level and buffers the full result.
func renderTemplate(filename string, raw json.RawMessage) ([]byte, error) {
	// UseNumber avoids rounding large integers or precise decimals during decoding.
	var variables map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&variables); err != nil {
		return nil, fmt.Errorf("decode config context for template %q: %w", filename, err)
	}
	if variables == nil {
		return nil, fmt.Errorf("config context for template %q must be a JSON object", filename)
	}

	// Reject undefined fields without HTML escaping. Trim control-block lines so
	// loops and conditionals do not add blank lines or indentation to the config.
	loader, err := loaders.NewFileSystemLoader(filepath.Dir(filename))
	if err != nil {
		return nil, fmt.Errorf("load template %q: %w", filename, err)
	}
	cfg := config.New()
	cfg.AutoEscape = false
	cfg.StrictUndefined = true
	cfg.TrimBlocks = true
	cfg.LeftStripBlocks = true
	cfg.KeepTrailingNewline = true
	template, err := exec.NewTemplate(filepath.Base(filename), cfg, loader, gonja.DefaultEnvironment)
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", filename, err)
	}

	// Keep output private until execution succeeds, then ensure a final newline.
	var output bytes.Buffer
	if err := template.Execute(&output, exec.NewContext(variables)); err != nil {
		return nil, fmt.Errorf("render template %q: %w", filename, err)
	}
	if !bytes.HasSuffix(output.Bytes(), []byte("\n")) {
		output.WriteByte('\n')
	}
	return output.Bytes(), nil
}
