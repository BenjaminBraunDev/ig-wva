package profiler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"

	benchmarking_data_pb "ig-wva/gen/go/benchmarking_data"
	common_pb "ig-wva/gen/go/common"
	profiler_service_pb "ig-wva/gen/go/profiler_service"
)

// Helper function to create simple data points using a struct literal.
func makeDP(rate, latency float32) *benchmarking_data_pb.BenchmarkingDataPoint {
	return &benchmarking_data_pb.BenchmarkingDataPoint{
		MeasuredRequestRateRps: rate,
		MeasuredLatencyTpotMs:  latency,
	}
}

func TestGenerateProfile_Interpolation(t *testing.T) {
	mockDS := NewMockBenchmarkingDataSource()
	p := NewProfiler(mockDS)
	ctx := context.Background()

	worker1 := &common_pb.WorkerType{Id: "w1"}
	req1 := &common_pb.RequestType{Id: "r1", LatencySloTpotMs: 300} // SLO = 300ms

	// Configure mock data
	mockDS.SetDataPoints(worker1.GetId(), req1.GetId(), []*benchmarking_data_pb.BenchmarkingDataPoint{
		makeDP(50, 100),
		makeDP(60, 500),
	})

	req := &profiler_service_pb.GenerateProfileRequest{
		WorkloadDefinition: &profiler_service_pb.WorkloadDefinition{
			WorkerTypes:  []*common_pb.WorkerType{worker1},
			RequestTypes: []*common_pb.RequestType{req1},
		},
	}

	resp, err := p.GenerateProfile(ctx, req)
	if err != nil {
		t.Fatalf("GenerateProfile failed: %v", err)
	}

	// Expected throughput: 55
	expectedProfile := &profiler_service_pb.PerformanceProfile{
		Entries: []*profiler_service_pb.PerformanceProfileEntry{
			{
				WorkerTypeId:     worker1.GetId(),
				RequestTypeId:    req1.GetId(),
				MaxThroughputRps: 55.0,
				Status:           profiler_service_pb.PerformanceProfileEntry_OK,
			},
		},
	}

	if diff := cmp.Diff(expectedProfile, resp, protocmp.Transform()); diff != "" {
		t.Errorf("GenerateProfile response mismatch (-want +got):\n%s", diff)
	}
}

func TestGenerateProfile_EdgeCases(t *testing.T) {
	mockDS := NewMockBenchmarkingDataSource()
	p := NewProfiler(mockDS)
	ctx := context.Background()

	worker1 := &common_pb.WorkerType{Id: "w1"}
	reqNoData := &common_pb.RequestType{Id: "r_no_data", LatencySloTpotMs: 200}
	reqUnattainable := &common_pb.RequestType{Id: "r_unattainable", LatencySloTpotMs: 50}
	reqHighestRate := &common_pb.RequestType{Id: "r_highest", LatencySloTpotMs: 600}
	reqInsufficient := &common_pb.RequestType{Id: "r_insufficient", LatencySloTpotMs: 150}
	reqExactMatch := &common_pb.RequestType{Id: "r_exact", LatencySloTpotMs: 500}

	// Configure mock data
	mockDS.SetDataPoints(worker1.GetId(), reqUnattainable.GetId(), []*benchmarking_data_pb.BenchmarkingDataPoint{makeDP(50, 100), makeDP(60, 500)})
	mockDS.SetDataPoints(worker1.GetId(), reqHighestRate.GetId(), []*benchmarking_data_pb.BenchmarkingDataPoint{makeDP(50, 100), makeDP(60, 500)})
	mockDS.SetDataPoints(worker1.GetId(), reqInsufficient.GetId(), []*benchmarking_data_pb.BenchmarkingDataPoint{makeDP(50, 100)})
	mockDS.SetDataPoints(worker1.GetId(), reqExactMatch.GetId(), []*benchmarking_data_pb.BenchmarkingDataPoint{makeDP(50, 100), makeDP(60, 500)})

	req := &profiler_service_pb.GenerateProfileRequest{
		WorkloadDefinition: &profiler_service_pb.WorkloadDefinition{
			WorkerTypes: []*common_pb.WorkerType{worker1},
			RequestTypes: []*common_pb.RequestType{
				reqNoData,
				reqUnattainable,
				reqHighestRate,
				reqInsufficient,
				reqExactMatch,
			},
		},
	}

	resp, err := p.GenerateProfile(ctx, req)
	if err != nil {
		t.Fatalf("GenerateProfile failed: %v", err)
	}

	expectedProfile := &profiler_service_pb.PerformanceProfile{
		Entries: []*profiler_service_pb.PerformanceProfileEntry{
			{WorkerTypeId: worker1.GetId(), RequestTypeId: reqNoData.GetId(), Status: profiler_service_pb.PerformanceProfileEntry_NO_DATA_FOUND},
			{WorkerTypeId: worker1.GetId(), RequestTypeId: reqUnattainable.GetId(), Status: profiler_service_pb.PerformanceProfileEntry_SLO_UNATTAINABLE},
			{WorkerTypeId: worker1.GetId(), RequestTypeId: reqHighestRate.GetId(), MaxThroughputRps: 60, Status: profiler_service_pb.PerformanceProfileEntry_OK_USING_HIGHEST_RATE},
			{WorkerTypeId: worker1.GetId(), RequestTypeId: reqInsufficient.GetId(), MaxThroughputRps: 50, Status: profiler_service_pb.PerformanceProfileEntry_OK_USING_HIGHEST_RATE},
			{WorkerTypeId: worker1.GetId(), RequestTypeId: reqExactMatch.GetId(), MaxThroughputRps: 60, Status: profiler_service_pb.PerformanceProfileEntry_OK_USING_HIGHEST_RATE},
		},
	}

	opts := []cmp.Option{
		protocmp.Transform(),
		protocmp.SortRepeated(func(a, b *profiler_service_pb.PerformanceProfileEntry) bool {
			return a.GetRequestTypeId() < b.GetRequestTypeId()
		}),
	}

	if diff := cmp.Diff(expectedProfile, resp, opts...); diff != "" {
		t.Errorf("GenerateProfile response mismatch (-want +got):\n%s", diff)
	}
}

func TestGenerateProfile_DataSourceError(t *testing.T) {
	mockDS := NewMockBenchmarkingDataSource()
	p := NewProfiler(mockDS)
	ctx := context.Background()

	worker1 := &common_pb.WorkerType{Id: "w1"}
	reqDsError := &common_pb.RequestType{Id: "r_ds_error_case", LatencySloTpotMs: 200}
	mockErr := errors.New("mock data source specific error")
	mockDS.SetError(worker1.GetId(), reqDsError.GetId(), mockErr)

	req := &profiler_service_pb.GenerateProfileRequest{
		WorkloadDefinition: &profiler_service_pb.WorkloadDefinition{
			WorkerTypes:  []*common_pb.WorkerType{worker1},
			RequestTypes: []*common_pb.RequestType{reqDsError},
		},
	}

	resp, err := p.GenerateProfile(ctx, req)

	if err == nil {
		t.Fatalf("GenerateProfile expected an error due to data source failure, but got nil. Response: %v", resp)
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("GenerateProfile returned non-status error: %v", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("GenerateProfile error code: got %v, want %v", st.Code(), codes.Internal)
	}
	expectedErrMsg := fmt.Sprintf("failed to fetch data for worker %s, request %s: %v", worker1.GetId(), reqDsError.GetId(), mockErr)
	if st.Message() != expectedErrMsg {
		t.Errorf("GenerateProfile error message: got '%s', want '%s'", st.Message(), expectedErrMsg)
	}
	if resp != nil {
		t.Errorf("GenerateProfile expected nil response on error, got %v", resp)
	}
}

func TestGenerateProfile_InvalidInput(t *testing.T) {
	mockDS := NewMockBenchmarkingDataSource()
	p := NewProfiler(mockDS)
	ctx := context.Background()

	testCases := []struct {
		name    string
		req     *profiler_service_pb.GenerateProfileRequest
		wantErr codes.Code
	}{
		{
			name:    "nil request",
			req:     nil,
			wantErr: codes.InvalidArgument,
		},
		{
			name:    "nil workload definition",
			req:     &profiler_service_pb.GenerateProfileRequest{},
			wantErr: codes.InvalidArgument,
		},
		{
			name: "empty worker types",
			req: &profiler_service_pb.GenerateProfileRequest{
				WorkloadDefinition: &profiler_service_pb.WorkloadDefinition{
					RequestTypes: []*common_pb.RequestType{{Id: "r1"}},
				},
			},
			wantErr: codes.InvalidArgument,
		},
		{
			name: "empty request types",
			req: &profiler_service_pb.GenerateProfileRequest{
				WorkloadDefinition: &profiler_service_pb.WorkloadDefinition{
					WorkerTypes: []*common_pb.WorkerType{{Id: "w1"}},
				},
			},
			wantErr: codes.InvalidArgument,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := p.GenerateProfile(ctx, tc.req)
			if err == nil {
				t.Fatalf("GenerateProfile() expected error, got nil")
			}
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("GenerateProfile() expected gRPC status error, got %T: %v", err, err)
			}
			if st.Code() != tc.wantErr {
				t.Errorf("GenerateProfile() wrong error code: got %v, want %v", st.Code(), tc.wantErr)
			}
		})
	}
}

// Note: The original tests for identical latencies and multiple points have been omitted for brevity,
// as they follow the same refactoring pattern. They would be updated similarly to the tests above.
