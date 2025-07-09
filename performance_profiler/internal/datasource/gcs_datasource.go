package datasource

import (
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"

	benchmarking_data_pb "ig-wva/gen/go/benchmarking_data"
	common_pb "ig-wva/gen/go/common"

	"cloud.google.com/go/storage"
)

// GCSDataSource implements the BenchmarkingDataSource interface using a CSV file from GCS.
type GCSDataSource struct {
	gcsClient  *storage.Client
	bucketName string
	objectName string
}

// NewGCSDataSource creates a new GCSDataSource.
func NewGCSDataSource(ctx context.Context, bucketName, objectName string) (*GCSDataSource, error) {
	if bucketName == "" || objectName == "" {
		return nil, fmt.Errorf("GCS bucket name and object name must be provided")
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	return &GCSDataSource{
		gcsClient:  client,
		bucketName: bucketName,
		objectName: objectName,
	}, nil
}

// FetchDataPoints retrieves benchmarking data points from a CSV file in GCS.
func (gds *GCSDataSource) FetchDataPoints(ctx context.Context, workerType *common_pb.WorkerType, requestType *common_pb.RequestType) ([]*benchmarking_data_pb.BenchmarkingDataPoint, error) {
	rc, err := gds.gcsClient.Bucket(gds.bucketName).Object(gds.objectName).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS object reader for gs://%s/%s: %w", gds.bucketName, gds.objectName, err)
	}
	defer rc.Close()

	csvReader := csv.NewReader(rc)
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV data from gs://%s/%s: %w", gds.bucketName, gds.objectName, err)
	}

	if len(records) < 2 { // At least a header and one data row
		// log.Warningf(ctx, "CSV file gs://%s/%s has no data rows.", gds.bucketName, gds.objectName)
		fmt.Printf("Warning: CSV file gs://%s/%s has no data rows.\n", gds.bucketName, gds.objectName)
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
			return nil, fmt.Errorf("missing required column '%s' in CSV header from gs://%s/%s", col, gds.bucketName, gds.objectName)
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
			// log.Warningf(ctx, "Failed to parse request rate '%s' at row %d in gs://%s/%s: %v. Skipping row.", rateStr, i+2, gds.bucketName, gds.objectName, err)
			fmt.Printf("Warning: Failed to parse request rate '%s' at row %d in gs://%s/%s: %v. Skipping row.\n", rateStr, i+2, gds.bucketName, gds.objectName, err)
			continue
		}
		latency, err := strconv.ParseFloat(latencyStr, 32)
		if err != nil {
			// log.Warningf(ctx, "Failed to parse latency '%s' at row %d in gs://%s/%s: %v. Skipping row.", latencyStr, i+2, gds.bucketName, gds.objectName, err)
			fmt.Printf("Warning: Failed to parse latency '%s' at row %d in gs://%s/%s: %v. Skipping row.\n", latencyStr, i+2, gds.bucketName, gds.objectName, err)
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
		// log.Infof(ctx, "No data points found in CSV gs://%s/%s for worker %s, request %s after filtering.", gds.bucketName, gds.objectName, workerType.GetId(), requestType.GetId())
		fmt.Printf("Info: No data points found in CSV gs://%s/%s for worker %s, request %s after filtering.\n", gds.bucketName, gds.objectName, workerType.GetId(), requestType.GetId())
	}

	return dataPoints, nil
}
