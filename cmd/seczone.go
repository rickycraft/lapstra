package cmd

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/Juniper/apstra-go-sdk/datacenter"
	"github.com/charmbracelet/huh/spinner"
	"rickycraft/lapstra/core"
)

func (l *LapstraCmd) RoutingZoneCreate() error {

	name, err := AskName("Routing zone name")
	if err != nil {
		return err
	}

	description, err := AskName("Routing zone description")
	if err != nil {
		return err
	}

	vid, err := AskVlan()
	if err != nil {
		return err
	}

	szs := make([]apstra.ObjectId, l.size)
	errors := make([]error, l.size)
	wg := sync.WaitGroup{}
	for idx, client := range l.clients {
		wg.Go(func() {
			sz, err := core.CreateSecurityZone(l.ctx, client, name, description, vid)
			if err != nil {
				errors[idx] = err
			}
			szs[idx] = sz
		})
	}

	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range szs {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else {
			l.PrintCommandResult(idx, "created zone with id %s", szs[idx])
		}
	}

	return nil

}

func (l *LapstraCmd) RoutingZoneDelete() error {

	zones, err := l.AskRoutingZone("Select routing zone")
	if err != nil {
		return err
	}

	count := 0
	for _, vn := range zones {
		if vn != nil {
			count++
		}
	}
	if count == 0 {
		return fmt.Errorf("no routing zone found")
	}

	desc := make([]string, l.size)
	for idx := range l.clients {
		if zones[idx] == nil {
			continue
		}
		desc[idx] = fmt.Sprintf("[%s] routing zone %s", l.labels[idx], zones[idx].Label)
	}
	confirm, err := AskConfirm("Are you sure you want to delete the routing zone?", desc)
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
			if zones[idx] == nil {
				return
			}
			id := zones[idx].ID()
			if id == nil {
				errors[idx] = fmt.Errorf("routing zone %q has no id", zones[idx].Label)
				return
			}
			err := client.DeleteSecurityZone(l.ctx, *id)
			if err != nil {
				errors[idx] = err
			}
		})
	}
	spinner.New().Action(func() { wg.Wait() }).Run()

	for idx := range zones {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		} else if zones[idx] != nil {
			l.PrintCommandResult(idx, "deleted routing zone %s", zones[idx].Label)
		}
	}

	return nil
}

func (l *LapstraCmd) RoutingZoneMap() LapstraInfo[*datacenter.SecurityZone] {

	zoneMap := make(LapstraInfo[*datacenter.SecurityZone])
	wg := sync.WaitGroup{}
	mut := sync.Mutex{}
	for idx, client := range l.clients {
		wg.Go(func() {
			zones, err := client.GetSecurityZones(l.ctx)
			if err != nil {
				return
			}
			mut.Lock()
			defer mut.Unlock()
			for _, zone := range zones {
				if len(zoneMap[zone.Label]) == 0 {
					zoneMap[zone.Label] = make([]*datacenter.SecurityZone, l.size)
				}
				zoneMap[zone.Label][idx] = &zone
			}
		})
	}

	wg.Wait()
	return zoneMap
}

func (l *LapstraCmd) RoutingZoneShow() error {

	var zoneMap LapstraInfo[*datacenter.SecurityZone]
	spinner.New().Action(func() { zoneMap = l.RoutingZoneMap() }).Run()

	results := make([]string, 0)
	for name, zones := range zoneMap {
		var sb strings.Builder
		fmt.Fprintf(&sb, "Name %s", name)
		for idx, zone := range zones {
			if zone == nil {
				continue
			}
			rt := "none"
			if zone.RouteTarget != nil {
				rt = *zone.RouteTarget
			}
			fmt.Fprintf(&sb, "\n\t[%s]\tRT %s", l.labels[idx], rt)
		}
		results = append(results, sb.String())
	}

	sort.Strings(results)
	err := ShowViewport(strings.Join(results, "\n"))
	if err != nil {
		return err
	}

	return nil
}
