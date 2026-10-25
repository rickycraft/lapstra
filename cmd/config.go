package cmd

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/huh/v2/spinner"
	"rickycraft/lapstra/core"
)

var (
	// Apstra/Junos emits compactable VLAN changes as a three-line block:
	// edit context, old members line, new members line.
	vlanEditBlockRE = regexp.MustCompile(`^(\[edit interfaces [^]]+ unit [0-9]+ family ethernet-switching vlan\])$`)
	vlanNameRE      = regexp.MustCompile(`\bvn([0-9]+)\b`)
)

func (l *LapstraCmd) CheckConfig() error {

	errors := l.Errors()
	wg := sync.WaitGroup{}

	for idx, client := range l.clients {
		wg.Go(func() {
			err := core.StartCommitCheck(l.ctx, client)
			if err != nil {
				errors[idx] = err
			}
		})
	}
	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range l.clients {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "config check started")
		}
	}

	// start polling with max timeout of 60 seconds
	ctx, cancel := context.WithTimeout(l.ctx, 5*time.Minute)
	defer cancel()

	for idx, client := range l.clients {
		// avoid checking blueprint with an error
		if errors[idx] != nil {
			continue
		}
		wg.Go(func() {
			// check every 5 second the status
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					// timeout exceeded
					errors[idx] = fmt.Errorf("timeout exceeded")
					return

				case <-ticker.C:
					// check the status
					res, err := core.GetCommitCheck(ctx, client)
					if err != nil {
						errors[idx] = err
						return
					}
					// l.BlueprintLogger(idx).Debugf("🔄⏳ config check %s %s", res.State, res.Validity)
					// wait if is still pending
					if res.State == core.StatePending {
						continue
					}
					// return if is success and fresh
					if res.State == core.StateSuccess && res.Validity == core.ValidityFresh {
						return
					}
					// return error otherwise
					errors[idx] = fmt.Errorf("config check failed - %s %s", res.State, res.Validity)
					return
				}
			}
		})
	}

	core.Logger().Info("config check started")
	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range l.clients {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "config check completed")
		}
	}

	return nil
}

func (l *LapstraCmd) ShowConfigDiff() error {

	errors := l.Errors()
	wg := sync.WaitGroup{}
	configDiff := make(map[string]string)

	for idx, client := range l.clients {
		wg.Go(func() {
			systems, err := core.GetLeafs(l.ctx, client)
			if err != nil {
				errors[idx] = err
				return
			}
			if len(systems) != 2 {
				l.BlueprintLogger(idx).Warnf("expected 2 systems, got %d", len(systems))
			}
			for _, system := range systems {
				res, err := core.GetDeviceDiff(l.ctx, client, system.ID)
				if err != nil {
					errors[idx] = err
					return
				}
				configDiff[system.Hostname] = compactVLANMemberDiff(res.Diff)
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()
	l.PrintCommandErrors(errors)
	for _, err := range errors {
		if err != nil {
			return err
		}
	}

	builder := strings.Builder{}
	systems := make([]string, 0, len(configDiff))
	for system := range configDiff {
		systems = append(systems, system)
	}
	sort.Strings(systems)
	for _, system := range systems {
		fmt.Fprintf(&builder, "[%s]%s\n", system, configDiff[system])
	}
	ShowViewport(builder.String())

	return nil
}

func (l *LapstraCmd) StartDeploy() error {

	if err := l.ShowConfigDiff(); err != nil {
		return err
	}

	description, err := AskString("Deploy description", "description", 256)
	if err != nil {
		return err
	}

	confirm, err := AskConfirm("Are you sure you want to deploy?", l.labels)
	if err != nil {
		return err
	}
	if !confirm {
		return fmt.Errorf("user aborted deployment")
	}

	errors := l.Errors()
	wg := sync.WaitGroup{}

	for idx, client := range l.clients {
		wg.Go(func() {
			err := core.DeployBlueprint(l.ctx, client, description)
			if err != nil {
				errors[idx] = err
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range l.clients {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "config deploy started: %s", description)
		}
	}

	return nil
}

func (l *LapstraCmd) RevertBlueprints() error {

	confirm, err := AskConfirm("Are you sure you want to revert uncommitted changes?", l.labels)
	if err != nil {
		return err
	}
	if !confirm {
		return fmt.Errorf("user aborted revert")
	}

	errors := l.Errors()
	wg := sync.WaitGroup{}

	for idx, client := range l.clients {
		wg.Go(func() {
			err := core.RevertBlueprint(l.ctx, client)
			if err != nil {
				errors[idx] = err
			}
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range l.clients {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "config reverted")
		}
	}

	return nil
}

// compactVLANMemberDiff preserves the original diff except for interface VLAN
// membership blocks, where the huge old/new member lists are replaced with only
// the VLAN names that were removed or added.
func compactVLANMemberDiff(diff string) string {
	lines := strings.SplitAfter(diff, "\n")
	var out strings.Builder

	for idx := 0; idx < len(lines); {
		rawContext := lines[idx]
		if rawContext == "" {
			idx++
			continue
		}

		context := strings.TrimSuffix(rawContext, "\n")
		match := vlanEditBlockRE.FindStringSubmatch(context)
		if match == nil || idx+2 >= len(lines) {
			out.WriteString(rawContext)
			idx++
			continue
		}

		oldLine := strings.TrimSuffix(lines[idx+1], "\n")
		newLine := strings.TrimSuffix(lines[idx+2], "\n")
		if !strings.HasPrefix(oldLine, "-") || !strings.HasPrefix(newLine, "+") || !strings.Contains(oldLine, "members [") || !strings.Contains(newLine, "members [") {
			out.WriteString(rawContext)
			idx++
			continue
		}

		oldVLANs := make(map[string]struct{})
		newVLANs := make(map[string]struct{})
		for _, vn := range vlanNameRE.FindAllString(oldLine, -1) {
			oldVLANs[vn] = struct{}{}
		}
		for _, vn := range vlanNameRE.FindAllString(newLine, -1) {
			newVLANs[vn] = struct{}{}
		}

		var removed, added []string
		for vn := range oldVLANs {
			if _, ok := newVLANs[vn]; !ok {
				removed = append(removed, vn)
			}
		}
		for vn := range newVLANs {
			if _, ok := oldVLANs[vn]; !ok {
				added = append(added, vn)
			}
		}
		sort.Slice(removed, func(i, j int) bool { return vlanNumber(removed[i]) < vlanNumber(removed[j]) })
		sort.Slice(added, func(i, j int) bool { return vlanNumber(added[i]) < vlanNumber(added[j]) })

		// If the old/new lists contain the same VLANs, keep the original three lines.
		if len(removed) == 0 && len(added) == 0 {
			out.WriteString(rawContext)
			out.WriteString(lines[idx+1])
			out.WriteString(lines[idx+2])
			idx += 3
			continue
		}

		out.WriteString(match[1])
		out.WriteString("\n")
		for _, vn := range removed {
			out.WriteString("- ")
			out.WriteString(vn)
			out.WriteString("\n")
		}
		for _, vn := range added {
			out.WriteString("+ ")
			out.WriteString(vn)
			out.WriteString("\n")
		}
		out.WriteString("removed: ")
		out.WriteString(strconv.Itoa(len(removed)))
		out.WriteString("  added: ")
		out.WriteString(strconv.Itoa(len(added)))
		out.WriteString("\n")
		idx += 3
	}

	return out.String()
}

func vlanNumber(vn string) int {
	num, err := strconv.Atoi(strings.TrimPrefix(vn, "vn"))
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return num
}
