package profiler

import (
	"context"
	"fmt"
	"sort"

	"ig-wva/performance_profiler/internal/datasource"

	profiler_service_pb "ig-wva/gen/go/profiler_service"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Profiler encapsulates the logic for generating performance profiles.
type Profiler struct {
	ds datasource.BenchmarkingDataSource
}

// NewProfiler creates a new Profiler instance.
func NewProfiler(ds datasource.BenchmarkingDataSource) *Profiler {
	return &Profiler{ds: ds}
}

// GenerateProfile computes the performance profile for the given workload.
func (p *Profiler) GenerateProfile(ctx context.Context, req *profiler_service_pb.GenerateProfileRequest) (*profiler_service_pb.PerformanceProfile, error) {
	workload := req.GetWorkloadDefinition()
	if workload == nil || len(workload.GetWorkerTypes()) == 0 || len(workload.GetRequestTypes()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "WorkloadDefinition with worker_types and request_types must be provided")
	}

	profile := &profiler_service_pb.PerformanceProfile{
		Entries: make([]*profiler_service_pb.PerformanceProfileEntry, 0, len(workload.GetWorkerTypes())*len(workload.GetRequestTypes())),
	}

	for _, workerType := range workload.GetWorkerTypes() {
		for _, requestType := range workload.GetRequestTypes() {
			// Initialize the entry using a struct literal for clarity.
			entry := &profiler_service_pb.PerformanceProfileEntry{
				WorkerTypeId:  workerType.GetId(),
				RequestTypeId: requestType.GetId(),
				Status:        profiler_service_pb.PerformanceProfileEntry_STATUS_UNSPECIFIED,
			}

			// Fetch relevant data points from the data source
			dataPoints, err := p.ds.FetchDataPoints(ctx, workerType, requestType)
			if err != nil {
				fmt.Printf("Error fetching data for worker %s, request %s: %v\n", workerType.GetId(), requestType.GetId(), err)
				return nil, status.Errorf(codes.Internal, "failed to fetch data for worker %s, request %s: %v", workerType.GetId(), requestType.GetId(), err)
			}

			if len(dataPoints) == 0 {
				entry.Status = profiler_service_pb.PerformanceProfileEntry_NO_DATA_FOUND
				profile.Entries = append(profile.Entries, entry)
				continue
			}

			sort.SliceStable(dataPoints, func(i, j int) bool {
				return dataPoints[i].GetMeasuredRequestRateRps() < dataPoints[j].GetMeasuredRequestRateRps()
			})

			sloMs := requestType.GetLatencySloTpotMs()
			lastPoint := dataPoints[len(dataPoints)-1]

			// Edge Case: SLO is lower than the latency at the lowest measured rate
			if sloMs < dataPoints[0].GetMeasuredLatencyTpotMs() {
				entry.Status = profiler_service_pb.PerformanceProfileEntry_SLO_UNATTAINABLE
				profile.Entries = append(profile.Entries, entry)
				continue
			}

			// Edge Case: SLO is higher than or equal to the latency at the highest measured rate
			if sloMs >= lastPoint.GetMeasuredLatencyTpotMs() {
				entry.MaxThroughputRps = lastPoint.GetMeasuredRequestRateRps()
				entry.Status = profiler_service_pb.PerformanceProfileEntry_OK_USING_HIGHEST_RATE
				profile.Entries = append(profile.Entries, entry)
				continue
			}

			// --- Interpolation Logic ---
			var r1, t1, r2, t2 float32
			foundBounds := false

			for i := 0; i < len(dataPoints)-1; i++ {
				p1 := dataPoints[i]
				p2 := dataPoints[i+1]

				if sloMs >= p1.GetMeasuredLatencyTpotMs() && sloMs < p2.GetMeasuredLatencyTpotMs() {
					r1 = p1.GetMeasuredRequestRateRps()
					t1 = p1.GetMeasuredLatencyTpotMs()
					r2 = p2.GetMeasuredRequestRateRps()
					t2 = p2.GetMeasuredLatencyTpotMs()

					if t2-t1 == 0 {
						entry.MaxThroughputRps = r1
					} else {
						// Linear Interpolation
						entry.MaxThroughputRps = r1 + (sloMs-t1)*(r2-r1)/(t2-t1)
					}

					entry.Status = profiler_service_pb.PerformanceProfileEntry_OK
					foundBounds = true
					break
				}
			}

			if !foundBounds && sloMs == lastPoint.GetMeasuredLatencyTpotMs() {
				entry.MaxThroughputRps = lastPoint.GetMeasuredRequestRateRps()
				entry.Status = profiler_service_pb.PerformanceProfileEntry_OK
				foundBounds = true
			}

			if !foundBounds {
				fmt.Printf("Warning: Could not interpolate for worker %s, request %s. SLO: %f, Points: %v\n", workerType.GetId(), requestType.GetId(), sloMs, dataPoints)
				entry.Status = profiler_service_pb.PerformanceProfileEntry_INTERNAL_ERROR
			}

			profile.Entries = append(profile.Entries, entry)
		}
	}

	return profile, nil
}
