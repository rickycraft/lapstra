package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/akamensky/argparse"
	"github.com/charmbracelet/huh"
	"rickycraft/lapstra/cli"
	"rickycraft/lapstra/cmd"
	"rickycraft/lapstra/core"
)

const (
	VERSION = 0.17
)

type envLookupFunc func(string) (string, bool)
type passwordPromptFunc func() (string, error)

func validateUsername(username string) error {
	if username == "admin" {
		return fmt.Errorf("username %q is forbidden; use a personal Apstra account", username)
	}
	return nil
}

func resolvePassword(lookupEnv envLookupFunc, prompt passwordPromptFunc) (string, error) {
	if password, ok := lookupEnv("LAPSTRA_PASS"); ok && password != "" {
		return password, nil
	}
	return prompt()
}

func promptPassword() (string, error) {
	var password string
	err := huh.NewInput().
		Title("Apstra password").
		EchoMode(huh.EchoModePassword).
		Value(&password).
		Run()
	return password, err
}

func main() {
	// Define command-line flags
	parser := argparse.NewParser(
		"lapstra",
		fmt.Sprintf("Cli to configure Apstra v%0.2f. Passwords are read from non-empty LAPSTRA_PASS or prompted securely if unset.", VERSION),
	)
	apiEndpoint := parser.String("a", "api", &argparse.Options{Required: true, Help: "Apstra API endpoint (e.g., https://your-apstra-server)"})
	username := parser.String("u", "user", &argparse.Options{Required: true, Help: "Personal Apstra username. The literal username admin is forbidden."})
	operation := parser.String("o", "operation", &argparse.Options{Required: false, Help: "Operation to perform: VniMerge, CtValidate, VlanCreate, CtExport, CtImport, Templates. If not provided, interactive mode is used."})
	err := parser.Parse(os.Args)
	if err != nil && !strings.Contains(err.Error(), "unknown arguments") {
		fmt.Println(parser.Usage(parser))
		fmt.Printf("error parsing args: %v\n", err)
		os.Exit(1)
	}
	if err := validateUsername(*username); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	core.SetApstraUser(*username)

	password, err := resolvePassword(os.LookupEnv, promptPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading password: %v\n", err)
		os.Exit(1)
	}

	interactive := *operation == ""
	selectBlueprintInteractively := interactive || *operation == "VlanCreate"
	var l *cmd.LapstraCmd
	if *operation == "CtExport" || *operation == "CtImport" || *operation == "Templates" {
		l, err = cmd.SelectSingleBlueprint(*apiEndpoint, *username, password)
	} else {
		l, err = cmd.SelectBlueprint(*apiEndpoint, *username, password, selectBlueprintInteractively)
	}
	if err != nil {
		if *operation == "Templates" && errors.Is(err, huh.ErrUserAborted) {
			return
		}
		core.Logger().Fatalf("error connecting blueprints - %v\n", err)
	}
	if *operation != "Templates" {
		fmt.Println()
	}

	// Check if an operation was provided
	if interactive {
		for {
			cmd, err := cmd.SelectCmd(l)
			if err != nil {
				core.Logger().Fatalf("error selecting command - %v\n", err)
			}
			err = cmd()
			if err != nil {
				core.Logger().Error(err)
			}
			fmt.Println()
		}
	} else {
		cli.RunCli(l, *operation)
	}
}
