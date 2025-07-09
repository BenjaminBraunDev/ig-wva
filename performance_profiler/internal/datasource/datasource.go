package datasource

import (
	"context"

	benchmarking_data_pb "ig-wva/gen/go/benchmarking_data"
	common_pb "ig-wva/gen/go/common"
)

// BenchmarkingDataSource defines the interface for retrieving benchmarking data.
type BenchmarkingDataSource interface {
	// FetchDataPoints retrieves benchmarking data points relevant to a specific
	// worker type and request type.
	// The implementation is responsible for querying the underlying storage (Spanner),
	// filtering based on the worker/request type attributes, extracting the
	// relevant (rate, latency) pairs, and returning them sorted by request rate.
	FetchDataPoints(ctx context.Context, workerType *common_pb.WorkerType, requestType *common_pb.RequestType) ([]*benchmarking_data_pb.BenchmarkingDataPoint, error)
}
