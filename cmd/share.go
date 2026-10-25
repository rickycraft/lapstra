package cmd

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/IBM/fp-go/array"
	"github.com/Juniper/apstra-go-sdk/apstra"
	"github.com/charmbracelet/log"
	"golang.org/x/sync/errgroup"
	"rickycraft/lapstra/core"
)

type Location int

const (
	DatacenterPair Location = iota
	Custom
	DC1
	DC2
	DC3
	DC4
)

func (l Location) String() string {
	return [...]string{"Datacenter Pair", "Custom", "DC1", "DC2", "DC3", "DC4"}[l]
}

func DcToLocation(dc string) Location {
	switch dc {
	case "DC1":
		return DC1
	case "DC2":
		return DC2
	case "DC3":
		return DC3
	case "DC4":
		return DC4
	default:
		return Custom
	}
}

type LapstraCmd struct {
	location  Location
	client    *apstra.Client
	clients   []*apstra.TwoStageL3ClosClient
	labels    []string
	locations []Location
	ctx       context.Context
	size      int
}

func labelToLocation(label string) Location {
	pattern := `(?:^|-)(DC[1-4])-C[0-9].*SPINE$`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(label)

	if len(matches) == 0 {
		return Custom
	} else {
		return DcToLocation(matches[1])
	}
}

func (l *LapstraCmd) Connect(client *apstra.Client, blueprints []apstra.BlueprintStatus, location Location) error {
	l.client = client
	// Select the speecific blueprint
	var ids []apstra.ObjectId
	switch location {
	case DatacenterPair:
		// Select all blueprints
		ids = array.Map(func(b apstra.BlueprintStatus) apstra.ObjectId { return b.Id })(blueprints)
	case Custom:
		// Make the user select the blueprints
		_ids, err := AskBlueprint(blueprints)
		if err != nil {
			return err
		}
		ids = _ids
	case DC1, DC2, DC3, DC4:
		// Select the blueprints for the selected location
		_ids := array.Filter(func(b apstra.BlueprintStatus) bool { return labelToLocation(b.Label) == location })(blueprints)
		ids = array.Map(func(b apstra.BlueprintStatus) apstra.ObjectId { return b.Id })(_ids)
	}

	// Allocate structs
	l.clients = make([]*apstra.TwoStageL3ClosClient, len(ids))
	l.labels = make([]string, len(ids))
	l.location = location
	l.locations = make([]Location, len(ids))
	l.ctx = context.Background()

	// Connect to each client
	eg := errgroup.Group{}
	for idx, id := range ids {
		eg.Go(func() error {
			client, err := client.NewTwoStageL3ClosClient(l.ctx, id)
			if err != nil {
				return err
			}
			l.clients[idx] = client
			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return err
	}

	// Populate labels
	for idx, c := range l.clients {
		id := c.Id()
		bpOptions := array.Filter(func(b apstra.BlueprintStatus) bool { return b.Id == id })(blueprints)
		if len(bpOptions) > 0 {
			l.labels[idx] = bpOptions[0].Label
			l.locations[idx] = labelToLocation(l.labels[idx])
		}
		l.PrintCommandResult(idx, "connected")
	}

	// Set various vars
	l.size = len(l.clients)

	return nil
}

func (l *LapstraCmd) Errors() []error {
	return make([]error, l.size)
}

func (l *LapstraCmd) BlueprintLogger(idx int) *log.Logger {
	return core.WithPrefix(l.labels[idx])
}

func (l *LapstraCmd) Logger() *log.Logger {
	return core.Logger()
}

func (l *LapstraCmd) PrintCommandResult(idx int, message string, args ...any) {
	l.BlueprintLogger(idx).Info(
		fmt.Sprintf(message, args...),
	)
}

func (l *LapstraCmd) PrintCommandError(idx int, err error) {
	l.BlueprintLogger(idx).Error(err)
}

func (l *LapstraCmd) PrintCommandErrors(errors []error) {
	for idx := range l.clients {
		if errors[idx] != nil {
			l.PrintCommandError(idx, errors[idx])
		}
	}
}

func (l *LapstraCmd) IsStretch() bool {
	if l.location == DatacenterPair {
		return true
	}
	if l.location == Custom {
		// Auto-detect: stretched if blueprints span multiple distinct locations
		seen := make(map[Location]bool)
		for _, loc := range l.locations {
			if loc != Custom {
				seen[loc] = true
			}
		}
		return len(seen) > 1
	}
	return false
}

func (l *LapstraCmd) Size() int {
	return l.size
}

func (l *LapstraCmd) Labels() []string {
	return l.labels
}

func (l *LapstraCmd) Clients() []*apstra.TwoStageL3ClosClient {
	return l.clients
}

func (l *LapstraCmd) Context(minutes ...int) (context.Context, context.CancelFunc) {
	timeout := 3 * time.Minute
	if len(minutes) > 0 && minutes[0] > 0 {
		timeout = time.Duration(minutes[0]) * time.Minute
	}
	return context.WithTimeout(l.ctx, timeout)
}

func (l *LapstraCmd) BpLocation(idx int) Location {
	return l.locations[idx]
}
