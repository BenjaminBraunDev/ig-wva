package profiler

import (
	"context"
	"fmt"

	benchmarking_data_pb "ig-wva/gen/go/benchmarking_data"
	common_pb "ig-wva/gen/go/common"
)

// MockBenchmarkingDataSource is a mock implementation of BenchmarkingDataSource for testing.
type MockBenchmarkingDataSource struct {
	// DataPointsToReturn holds the data points the mock should return for specific calls.
	// Key: A unique string representing the worker/request type combination (e.g., "worker1:requestA").
	// Value: The slice of data points to return for that key.
	DataPointsToReturn map[string][]*benchmarking_data_pb.BenchmarkingDataPoint

	// ErrorToReturn holds errors the mock should return for specific calls.
	// Key: A unique string representing the worker/request type combination.
	// Value: The error to return.
	ErrorToReturn map[string]error

	// DefaultDataPoints are returned if no specific entry is found in DataPointsToReturn.
	DefaultDataPoints []*benchmarking_data_pb.BenchmarkingDataPoint
	// DefaultError is returned if no specific entry is found in ErrorToReturn.
	DefaultError error
}

// NewMockBenchmarkingDataSource creates a new mock data source.
func NewMockBenchmarkingDataSource() *MockBenchmarkingDataSource {
	return &MockBenchmarkingDataSource{
		DataPointsToReturn: make(map[string][]*benchmarking_data_pb.BenchmarkingDataPoint),
		ErrorToReturn:      make(map[string]error),
	}
}

// FetchDataPoints implements the BenchmarkingDataSource interface for the mock.
func (m *MockBenchmarkingDataSource) FetchDataPoints(ctx context.Context, workerType *common_pb.WorkerType, requestType *common_pb.RequestType) ([]*benchmarking_data_pb.BenchmarkingDataPoint, error) {
	key := fmt.Sprintf("%s:%s", workerType.GetId(), requestType.GetId()) // Use GetId() for safety

	if err, exists := m.ErrorToReturn[key]; exists {
		return nil, err
	}
	if m.DefaultError != nil {
		return nil, m.DefaultError
	}

	if data, exists := m.DataPointsToReturn[key]; exists {
		// Return a copy to prevent tests from modifying the mock's internal state
		dataCopy := make([]*benchmarking_data_pb.BenchmarkingDataPoint, len(data))
		copy(dataCopy, data)
		return dataCopy, nil
	}

	if m.DefaultDataPoints != nil {
		dataCopy := make([]*benchmarking_data_pb.BenchmarkingDataPoint, len(m.DefaultDataPoints))
		copy(dataCopy, m.DefaultDataPoints)
		return dataCopy, nil
	}

	// Default behavior if nothing is configured: return empty slice, no error
	return []*benchmarking_data_pb.BenchmarkingDataPoint{}, nil
}

// SetDataPoints sets the data points to return for a specific worker and request type.
func (m *MockBenchmarkingDataSource) SetDataPoints(workerID, requestID string, data []*benchmarking_data_pb.BenchmarkingDataPoint) {
	key := fmt.Sprintf("%s:%s", workerID, requestID)
	m.DataPointsToReturn[key] = data
}

// SetError sets the error to return for a specific worker and request type.
func (m *MockBenchmarkingDataSource) SetError(workerID, requestID string, err error) {
	key := fmt.Sprintf("%s:%s", workerID, requestID)
	m.ErrorToReturn[key] = err
}
