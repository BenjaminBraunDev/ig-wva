package datasource

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	benchmarking_data_pb "ig-wva/gen/go/benchmarking_data"
	common_pb "ig-wva/gen/go/common"
)

// FileDataSource implements the BenchmarkingDataSource interface using a local CSV file.
type FileDataSource struct {
	filePath string
}

// NewFileDataSource creates a new FileDataSource.
// It returns an error if the path is empty or the file doesn't exist.
func NewFileDataSource(filePath string) (*FileDataSource, error) {
	if filePath == "" {
		return nil, fmt.Errorf("local CSV file path must be provided")
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	return &FileDataSource{
		filePath: filePath,
	}, nil
}

// FetchDataPoints retrieves benchmarking data points from a local CSV file.
func (fds *FileDataSource) FetchDataPoints(ctx context.Context, workerType *common_pb.WorkerType, requestType *common_pb.RequestType) ([]*benchmarking_data_pb.BenchmarkingDataPoint, error) {
	file, err := os.Open(fds.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", fds.filePath, err)
	}
	defer file.Close()

	csvReader := csv.NewReader(file)
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV data from %s: %w", fds.filePath, err)
	}

	if len(records) < 2 { // At least a header and one data row
		fmt.Printf("Warning: CSV file %s has no data rows.\n", fds.filePath)
		return []*benchmarking_data_pb.BenchmarkingDataPoint{}, nil
	}

	header := records[0]
	columnIndex := make(map[string]int)
	for i, colName := range header {
		columnIndex[strings.TrimSpace(colName)] = i
	}

	// Verify required columns exist
	requiredCols := []string{"accelerator_type", "input_range", "output_range", "metrics_request_rate", "metrics_p90_per_output_token_latency_mean"}
	for _, col := range requiredCols {
		if _, ok := columnIndex[col]; !ok {
			return nil, fmt.Errorf("missing required column '%s' in CSV header from %s", col, fds.filePath)
		}
	}

	var dataPoints []*benchmarking_data_pb.BenchmarkingDataPoint

	for i, row := range records[1:] { // Skip header row
		csvAcceleratorType := strings.TrimSpace(row[columnIndex["accelerator_type"]])
		csvInputBucket := strings.TrimSpace(row[columnIndex["input_range"]])
		csvOutputBucket := strings.TrimSpace(row[columnIndex["output_range"]])

		// Filter based on workerType and requestType
		if csvAcceleratorType != workerType.GetAcceleratorType() ||
			csvInputBucket != requestType.GetInputSizeBucket() ||
			csvOutputBucket != requestType.GetOutputSizeBucket() {
			continue
		}

		rateStr := strings.TrimSpace(row[columnIndex["metrics_request_rate"]])
		latencyStr := strings.TrimSpace(row[columnIndex["metrics_p90_per_output_token_latency_mean"]])

		rate, err := strconv.ParseFloat(rateStr, 32)
		if err != nil {
			fmt.Printf("Warning: Failed to parse request rate '%s' at row %d in %s: %v. Skipping row.\n", rateStr, i+2, fds.filePath, err)
			continue
		}
		latency, err := strconv.ParseFloat(latencyStr, 32)
		if err != nil {
			fmt.Printf("Warning: Failed to parse latency '%s' at row %d in %s: %v. Skipping row.\n", latencyStr, i+2, fds.filePath, err)
			continue
		}

		dp := &benchmarking_data_pb.BenchmarkingDataPoint{
			MeasuredRequestRateRps: float32(rate),
			MeasuredLatencyTpotMs:  float32(latency),
			InputSizeBucket:        requestType.GetInputSizeBucket(),
			OutputSizeBucket:       requestType.GetOutputSizeBucket(),
			AcceleratorType:        workerType.GetAcceleratorType(),
			AcceleratorCount:       workerType.GetAcceleratorCount(),
			ModelName:              workerType.GetModelName(),
			ModelServerType:        workerType.GetModelServerType(),
			ModelServerImage:       workerType.GetModelServerImage(),
		}

		dataPoints = append(dataPoints, dp)
	}

	sort.SliceStable(dataPoints, func(i, j int) bool {
		return dataPoints[i].GetMeasuredRequestRateRps() < dataPoints[j].GetMeasuredRequestRateRps()
	})

	if len(dataPoints) == 0 {
		fmt.Printf("Info: No data points found in CSV %s for worker %s, request %s after filtering.\n", fds.filePath, workerType.GetId(), requestType.GetId())
	}

	return dataPoints, nil
}
