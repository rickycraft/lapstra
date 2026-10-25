package cli

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/Juniper/apstra-go-sdk/apstra"
	"rickycraft/lapstra/core"
)

type NewCtVlanMulti struct {
	name string
	vnis []uint32
}

func ReadCsvBlukCtVlanMulti(path string) ([]NewCtVlanMulti, error) {
	rows, err := core.ReadCSVFile(path)
	if err != nil {
		return nil, err
	}

	// Slice to hold the parsed records.
	var records []NewCtVlanMulti

	start := 0
	if !bulkCTFirstRowIsData(rows[0]) {
		start = 1
	}
	for _, row := range rows[start:] {

		// Skip rows with no data.
		if len(row) == 0 {
			continue
		}

		// Create a new record with the name from the first column.
		record := NewCtVlanMulti{
			name: row[0],
			vnis: []uint32{},
		}

		// Parse each VNI value from the remaining columns.
		for _, vniStr := range row[1:] {
			// Skip empty fields if any.
			if vniStr == "" {
				continue
			}

			// Convert the CSV string to an integer.
			num, err := strconv.Atoi(vniStr)
			if err != nil {
				return nil, fmt.Errorf("error converting VNI %q: %w", vniStr, err)
			}
			record.vnis = append(record.vnis, uint32(num))
		}

		records = append(records, record)
	}

	// Example output of the parsed records.
	for _, rec := range records {
		fmt.Printf("Name: %s, VNIs: %v\n", rec.name, rec.vnis)
	}

	return records, nil
}

func bulkCTFirstRowIsData(row []string) bool {
	if len(row) < 2 {
		return false
	}
	foundVNI := false
	for _, vniText := range row[1:] {
		if vniText == "" {
			continue
		}
		if _, err := strconv.Atoi(vniText); err != nil {
			return false
		}
		foundVNI = true
	}
	return foundVNI
}

func CreateBulkCtVlanMulti(clients []*apstra.TwoStageL3ClosClient, cs []NewCtVlanMulti) error {

	ctx := context.Background()

	for _, c := range cs {

		wg := sync.WaitGroup{}
		errs := make([]error, len(clients))
		var failed atomic.Bool
		failed.Store(false)
		for idx := range clients {
			wg.Go(func() {
				err := CreateCtVlanMulti(ctx, clients[idx], c)
				if err != nil {
					errs[idx] = err
					failed.Store(true)
				}
			})
		}
		wg.Wait()

		if failed.Load() {
			for idx, err := range errs {
				core.Logger().Errorf("[%d] %s - %v", idx, c.name, err)
			}
			return fmt.Errorf("failed creating %s", c.name)
		} else {
			core.Logger().Infof("Created %s with VNIs %v", c.name, c.vnis)
		}

	}

	return nil
}

func CreateCtVlanMulti(ctx context.Context, client *apstra.TwoStageL3ClosClient, c NewCtVlanMulti) error {
	vnIds, err := SearchVnis(ctx, client, c.vnis)
	if err != nil {
		return err
	}
	// Create the virtual networks
	_, err = core.CreateConnectivityTemplateVlans(ctx, client, c.name, vnIds)
	if err != nil {
		return err
	}

	return nil
}

func SearchVnis(ctx context.Context, client *apstra.TwoStageL3ClosClient, vnis []uint32) ([]apstra.ObjectId, error) {

	vns, err := client.GetAllVirtualNetworks(ctx)
	if err != nil {
		return nil, err
	}

	vniMap := make(map[uint32]apstra.ObjectId)
	for _, vn := range vns {
		vniMap[*vn.Data.VnId] = vn.Id
	}

	vnIds := make([]apstra.ObjectId, len(vnis))
	for idx, vni := range vnis {
		if i, ok := vniMap[vni]; ok {
			vnIds[idx] = i
		} else {
			return nil, fmt.Errorf("vni %d not found", vni)
		}
	}

	return vnIds, nil
}
