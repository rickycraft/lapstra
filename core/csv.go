package core

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ReadCSVFile reads all records from path, trims leading field whitespace, and
// does not impose a fixed record width.
func ReadCSVFile(path string) ([][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open CSV %q: %w", path, err)
	}

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	records, readErr := reader.ReadAll()
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read CSV %q: %w", path, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close CSV %q: %w", path, closeErr)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("CSV %q is empty", path)
	}
	return records, nil
}

// SkipOptionalCSVHeader removes a matching first row and returns the CSV line
// number of the first data row. Header fields are trimmed and case-insensitive.
func SkipOptionalCSVHeader(records [][]string, header []string) ([][]string, int) {
	if len(records) == 0 || len(records[0]) != len(header) {
		return records, 1
	}
	for idx := range header {
		if !strings.EqualFold(strings.TrimSpace(records[0][idx]), strings.TrimSpace(header[idx])) {
			return records, 1
		}
	}
	return records[1:], 2
}

// WriteCSVFile writes all records to path and reports writer, flush, and close errors.
func WriteCSVFile(path string, records [][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create CSV %q: %w", path, err)
	}

	writer := csv.NewWriter(file)
	writer.WriteAll(records)
	writer.Flush()
	writeErr := writer.Error()
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write CSV %q: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close CSV %q: %w", path, closeErr)
	}
	return nil
}

// CSVFilesInDir returns regular .csv files in dir in deterministic filename order.
func CSVFilesInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0)
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".csv") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}
