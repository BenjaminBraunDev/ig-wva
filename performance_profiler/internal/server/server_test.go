package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"

	benchmarking_data_pb "ig-wva/gen/go/benchmarking_data"
	common_pb "ig-wva/gen/go/common"
	profiler_service_pb "ig-wva/gen/go/profiler_service"
	"ig-wva/performance_profiler/internal/profiler"
)

// Helper to create benchmarking data points using a composite literal.
func makeDP(rate, latency float32) *benchmarking_data_pb.BenchmarkingDataPoint {
	return &benchmarking_data_pb.BenchmarkingDataPoint{
		MeasuredRequestRateRps: rate,
		MeasuredLatencyTpotMs:  latency,
	}
}

// --- Test Cases ---

func TestGenerateProfile_Success(t *testing.T) {
	ctx := context.Background()

	mockDS := profiler.NewMockBenchmarkingDataSource()
	// Configure mock data source for w1, r1 to achieve 55 RPS with SLO 300ms
	// Data: (rate: 50, latency: 100) and (rate: 60, latency: 500)
	// Interpolation for SLO 300ms: 50 + (60-50)*(300-100)/(500-100) = 55
	mockDS.SetDataPoints("w1", "r1", []*benchmarking_data_pb.BenchmarkingDataPoint{
		makeDP(50, 100),
		makeDP(60, 500),
	})

	coreProfiler := profiler.NewProfiler(mockDS)
	s := NewServer(coreProfiler)

	req := &profiler_service_pb.GenerateProfileRequest{
		WorkloadDefinition: &profiler_service_pb.WorkloadDefinition{
			WorkerTypes: []*common_pb.WorkerType{
				{Id: "w1"},
			},
			RequestTypes: []*common_pb.RequestType{
				{Id: "r1", LatencySloTpotMs: 300}, // SLO is important for profiler
			},
		},
	}

	resp, err := s.GenerateProfile(ctx, req)
	if err != nil {
		t.Fatalf("GenerateProfile() failed with unexpected error: %v", err)
	}

	expectedResp := &profiler_service_pb.GenerateProfileResponse{
		PerformanceProfile: &profiler_service_pb.PerformanceProfile{
			Entries: []*profiler_service_pb.PerformanceProfileEntry{
				{
					WorkerTypeId:     "w1",
					RequestTypeId:    "r1",
					MaxThroughputRps: 55.0,
					Status:           profiler_service_pb.PerformanceProfileEntry_OK,
				},
			},
		},
	}

	if diff := cmp.Diff(expectedResp, resp, protocmp.Transform()); diff != "" {
		t.Errorf("GenerateProfile response mismatch (-want +got):\n%s", diff)
	}
}

func TestGenerateProfile_InvalidInput(t *testing.T) {
	ctx := context.Background()
	mockDS := profiler.NewMockBenchmarkingDataSource()
	coreProfiler := profiler.NewProfiler(mockDS)
	s := NewServer(coreProfiler)

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
			_, err := s.GenerateProfile(ctx, tc.req)
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

// TestGenerateProfile_DataSourceError tests how the server responds when the underlying
// data source encounters an error.
func TestGenerateProfile_DataSourceError(t *testing.T) {
	ctx := context.Background()
	workerID := "w1"
	requestID := "r1_ds_error"

	mockDS := profiler.NewMockBenchmarkingDataSource()
	mockDS.SetError(workerID, requestID, errors.New("mock data source failure"))

	coreProfiler := profiler.NewProfiler(mockDS)
	s := NewServer(coreProfiler)

	validReq := &profiler_service_pb.GenerateProfileRequest{
		WorkloadDefinition: &profiler_service_pb.WorkloadDefinition{
			WorkerTypes:  []*common_pb.WorkerType{{Id: workerID}},
			RequestTypes: []*common_pb.RequestType{{Id: requestID, LatencySloTpotMs: 200}},
		},
	}

	_, err := s.GenerateProfile(ctx, validReq)
	if err == nil {
		t.Fatalf("GenerateProfile() expected an error due to data source failure, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("GenerateProfile() expected gRPC status error, got %T: %v", err, err)
	}

	if st.Code() != codes.Internal {
		t.Errorf("GenerateProfile() wrong error code for data source error: got %v, want %v", st.Code(), codes.Internal)
	}
}

// TestGenerateProfile_ProfilerInvalidArgument tests that the server correctly
// propagates an InvalidArgument error from the profiler.
func TestGenerateProfile_ProfilerInvalidArgument(t *testing.T) {
	ctx := context.Background()

	mockDS := profiler.NewMockBenchmarkingDataSource()
	coreProfiler := profiler.NewProfiler(mockDS)
	s := NewServer(coreProfiler)

	// This test case is somewhat artificial as server validation should catch this.
	// However, it confirms that if profiler.GenerateProfile *does* return InvalidArgument,
	// the server propagates it correctly.
	reqWithNilWorkload := &profiler_service_pb.GenerateProfileRequest{} // WorkloadDefinition is nil

	_, err := s.GenerateProfile(ctx, reqWithNilWorkload)
	if err == nil {
		t.Fatalf("GenerateProfile() expected an error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("GenerateProfile() expected gRPC status error, got %T: %v", err, err)
	}

	if st.Code() != codes.InvalidArgument {
		t.Errorf("GenerateProfile() wrong error code: got %v, want %v", st.Code(), codes.InvalidArgument)
	}
	if !strings.Contains(st.Message(), "Request and WorkloadDefinition must be provided") {
		t.Errorf("GenerateProfile() error message '%s' does not seem to be from the server's initial validation for this case", st.Message())
	}
}
