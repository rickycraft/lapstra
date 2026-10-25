package cli

import (
	"fmt"
	"os"

	"rickycraft/lapstra/cmd"
)

func RunCli(l *cmd.LapstraCmd, operation string) {
	switch operation {
	case "VniMerge":
		RunMergeVlan(l)
	case "CtValidate":
		RunCtValidate(l)
	case "VlanCreate":
		RunVlanCreate(l)
	case "CtExport":
		RunCtExport(l)
	case "CtImport":
		RunCtImport(l)
	case "Templates":
		RunTemplates(l)
	default:
		fmt.Printf("Unknown operation: %s\n", operation)
		os.Exit(1)
	}
}
