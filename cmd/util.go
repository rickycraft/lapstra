package cmd

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Juniper/apstra-go-sdk/datacenter"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
)

const DefaultTimeout = 20 * time.Second

type LapstraInfo[T any] map[string][]T

func AskNumber(title string, prompt string, min uint64, max uint64) (uint64, error) {

	var strval string
	err := huh.NewInput().
		Title(title).
		Prompt(prompt + " ").
		Validate(func(i string) error {
			num, err := strconv.ParseUint(i, 10, 64)
			if err != nil {
				return err
			}
			if num < min || num > max {
				return fmt.Errorf("number must be between %d and %d", min, max)
			}
			return nil
		}).Value(&strval).
		Run()
	if err != nil {
		return 0, err
	}
	num, err := strconv.ParseUint(strval, 10, 64)
	if err != nil {
		return 0, err
	}
	return num, nil
}

func AskVlan() (uint16, error) {

	num, err := AskNumber("Enter vlan number", "vid", 1, 4094)
	if err != nil {
		return 0, err
	}
	return uint16(num), nil
}

func AskString(title string, prompt string, limit int) (string, error) {

	var val string
	err := huh.NewInput().
		Title(title).
		Prompt(prompt + ": ").
		Validate(func(i string) error {
			if len(i) == 0 {
				return fmt.Errorf("%s cannot be empty", prompt)
			}
			if len(i) > limit {
				return fmt.Errorf("%s cannot be longer than %d characters", prompt, limit)
			}
			return nil
		}).Value(&val).
		Run()
	if err != nil {
		return "", err
	}
	return val, nil
}

func AskName(title string) (string, error) {

	return AskString(title, "name", 128)
}

func AskIpSubnet(title string) (net.IP, *net.IPNet, error) {
	var netStr string

	err := huh.NewInput().
		Title(title).
		Prompt("ip (x.x.x.x/y): ").
		Validate(func(s string) error {
			_, _, err := net.ParseCIDR(s)
			if err != nil {
				return fmt.Errorf("ip must be in CIDR format")
			}
			return nil
		}).
		Value(&netStr).
		Run()
	if err != nil {
		return nil, nil, err
	}
	ip, network, err := net.ParseCIDR(netStr)
	if err != nil {
		return nil, nil, err
	}
	return ip, network, nil
}

func NetowrkCIDR(mask *net.IPNet) int {
	prefixLength, bits := mask.Mask.Size()
	if bits == 0 {
		return 0
	}
	return prefixLength
}

func AskIp(title string) (net.IP, error) {
	var ipStr string

	err := huh.NewInput().
		Title(title).
		Prompt("ip (x.x.x.x): ").
		Validate(func(s string) error {
			ip := net.ParseIP(s)
			if ip == nil {
				return fmt.Errorf("invalid IP address")
			}
			return nil
		}).
		Value(&ipStr).
		Run()
	if err != nil {
		return nil, err
	}
	return net.ParseIP(ipStr), nil
}

func AskConfirm(title string, description []string) (bool, error) {

	sb := strings.Builder{}
	for _, line := range description {
		if len(line) > 0 {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	var val bool
	err := huh.NewConfirm().
		Title(title).
		Description(sb.String()).
		Affirmative("Yes").
		Negative("No").
		Value(&val).
		Run()
	if err != nil {
		return false, err
	}
	return val, nil
}

func (l *LapstraCmd) AskRoutingZone(title string) ([]*datacenter.SecurityZone, error) {

	// Find all security zones
	var zoneMap LapstraInfo[*datacenter.SecurityZone]
	spinner.New().Action(func() { zoneMap = l.RoutingZoneMap() }).Run()

	// Ask the user from a list
	zoneNames := make([]string, 0, len(zoneMap))
	for name := range zoneMap {
		zoneNames = append(zoneNames, name)
	}
	sort.Strings(zoneNames)

	zoneOptions := make([]huh.Option[string], 0)
	var selectedZoneName string
	for _, name := range zoneNames {
		zoneOptions = append(zoneOptions, huh.Option[string]{
			Value: name,
			Key:   name,
		})
	}
	err := huh.NewSelect[string]().
		Title(title).
		Options(zoneOptions...).
		Value(&selectedZoneName).
		Height(10).
		Run()
	if err != nil {
		return nil, err
	}

	// Find the selected zone
	return zoneMap[selectedZoneName], nil
}
