package cli

import (
	"fmt"
	"os"
	"sync"

	"github.com/charmbracelet/huh/spinner"
	"rickycraft/lapstra/cmd"
	"rickycraft/lapstra/core"
)

func RunCtValidate(l *cmd.LapstraCmd) {
	if err := ctValidate(l); err != nil {
		os.Exit(1)
	}
}

func ctValidate(l *cmd.LapstraCmd) error {
	errors := l.Errors()
	wg := sync.WaitGroup{}
	ctx, cancel := l.Context()
	defer cancel()

	for idx, client := range l.Clients() {
		wg.Go(func() {
			labels, err := core.GetCtObj(ctx, client)
			if err != nil {
				errors[idx] = err
				return
			}

			if len(labels) == 0 {
				l.PrintCommandResult(idx, "no matching CTs")
				return
			}
			for _, label := range labels {
				l.PrintCommandResult(idx, "%s", label)
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()
	l.PrintCommandErrors(errors)

	for _, err := range errors {
		if err != nil {
			return fmt.Errorf("failed to validate CTs")
		}
	}
	return nil
}
