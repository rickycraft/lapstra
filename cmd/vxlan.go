package cmd

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"rickycraft/lapstra/core"
)

func AskVnis() ([]uint32, error) {
	var input string
	parse := func(input string) ([]uint32, error) {
		fields := strings.FieldsFunc(input, func(r rune) bool {
			return r == ',' || unicode.IsSpace(r)
		})
		if len(fields) == 0 {
			return nil, fmt.Errorf("enter at least one VNI")
		}
		vnis := make([]uint32, 0, len(fields))
		seen := make(map[uint32]bool, len(fields))
		for _, field := range fields {
			num, err := strconv.ParseUint(field, 10, 32)
			if err != nil || num == 0 {
				return nil, fmt.Errorf("VNI %q must be a decimal number between 1 and %d", field, uint64(math.MaxUint32))
			}
			vni := uint32(num)
			if !seen[vni] {
				vnis = append(vnis, vni)
				seen[vni] = true
			}
		}
		return vnis, nil
	}
	err := huh.NewInput().
		Title("Enter VXLAN numbers (comma or whitespace separated)").
		Prompt("vnis ").
		Validate(func(input string) error {
			_, err := parse(input)
			return err
		}).Value(&input).
		Run()
	if err != nil {
		return nil, err
	}
	return parse(input)
}

func AskVni() (uint32, error) {

	num, err := AskNumber("Enter vxlan number", "vni", 1, math.MaxUint32)
	if err != nil {
		return 0, err
	}
	return uint32(num), nil
}

func (l *LapstraCmd) vnDelete(vns []*apstra.VirtualNetwork) error {
	count := 0
	for _, vn := range vns {
		if vn != nil {
			count++
		}
	}
	if count == 0 {
		return fmt.Errorf("no virtual networks found")
	}

	desc := make([]string, l.size)
	for idx := range l.clients {
		if vns[idx] == nil {
			continue
		}
		label := l.labels[idx]
		desc[idx] = fmt.Sprintf("[%s] virtual network %s", label, vns[idx].Data.Label)
	}
	confirm, err := AskConfirm("Are you sure you want to delete the virtual network?", desc)
	if err != nil {
		return err
	}
	if !confirm {
		return fmt.Errorf("user aborted deletion")
	}

	errors := l.Errors()
	wg := sync.WaitGroup{}
	for idx, client := range l.clients {
		wg.Go(func() {
			if vns[idx] == nil {
				return
			}
			err := client.DeleteVirtualNetwork(l.ctx, vns[idx].Id)
			if err != nil {
				errors[idx] = err
			}
		})
	}
	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range vns {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else if vns[idx] != nil {
			l.PrintCommandResult(idx, "deleted virtual network %s vni %d", vns[idx].Data.Label, *vns[idx].Data.VnId)
		}
	}

	return nil
}

func (l *LapstraCmd) VniDelete() error {

	vni, err := AskVni()
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	vns := make([]*apstra.VirtualNetwork, l.Size())
	errors := l.Errors()
	for idx, client := range l.clients {
		wg.Go(func() {
			vn, err := core.GetVirtualNetworkVni(l.ctx, client, vni)
			if err != nil {
				errors[idx] = err
			} else {
				vns[idx] = vn
			}
		})
	}
	spinner.New().Action(func() { wg.Wait() }).Run()

	return l.vnDelete(vns)
}

func (l *LapstraCmd) VniPoolShow() error {

	var vniPools []*apstra.VniPool
	errors := l.Errors()
	wg := sync.WaitGroup{}
	for idx := range l.clients {
		wg.Go(func() {
			dcName := core.MapLocationVnPoolName(l.locations[idx].String())
			pool, err := l.client.GetVniPoolByName(l.ctx, dcName)
			if err != nil {
				errors[idx] = err
			} else {
				vniPools = append(vniPools, pool)
			}
		})
	}
	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range vniPools {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "vni pool %s / %d - %d",
				vniPools[idx].DisplayName, vniPools[idx].Ranges[0].First, vniPools[idx].Ranges[0].Last,
			)
		}
	}

	return nil
}
