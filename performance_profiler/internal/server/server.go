package server

import (
	"context"

	profiler_service_pb "ig-wva/gen/go/profiler_service"
	"ig-wva/performance_profiler/internal/profiler"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the PerformanceProfileGenerator gRPC service.
type Server struct {
	profiler_service_pb.UnimplementedPerformanceProfileGeneratorServer // Embed for forward compatibility
	profiler                                                           *profiler.Profiler
}

// NewServer creates a new gRPC server instance.
func NewServer(p *profiler.Profiler) *Server {
	if p == nil {
		panic("Profiler cannot be nil") // Or handle error appropriately
	}
	return &Server{
		profiler: p,
	}
}

// GenerateProfile handles incoming gRPC requests to generate a performance profile.
// The signature is modified to match the generated gRPC server interface.
func (s *Server) GenerateProfile(ctx context.Context, req *profiler_service_pb.GenerateProfileRequest) (*profiler_service_pb.GenerateProfileResponse, error) {
	// Basic input validation
	if req == nil || req.GetWorkloadDefinition() == nil {
		return nil, status.Error(codes.InvalidArgument, "Request and WorkloadDefinition must be provided")
	}
	if len(req.GetWorkloadDefinition().GetWorkerTypes()) == 0 || len(req.GetWorkloadDefinition().GetRequestTypes()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "WorkloadDefinition must contain at least one worker_type and one request_type")
	}

	// Log the incoming request details (consider logging level and PII)
	// log.Infof(ctx, "Received GenerateProfile request for %d worker types and %d request types", len(req.WorkloadDefinition.WorkerTypes), len(req.WorkloadDefinition.RequestTypes))

	// Call the core profiler logic
	profile, err := s.profiler.GenerateProfile(ctx, req)
	if err != nil {
		// Log the error from the profiler
		// log.Errorf(ctx, "Error generating profile: %v", err)

		// profiler.GenerateProfile is expected to return gRPC status errors (or nil).
		return nil, err // Return existing gRPC status error directly
	}

	// Log successful completion (consider logging profile size or key details)
	// log.Infof(ctx, "Successfully generated profile with %d entries", len(profile.Entries))

	// Create and return the response object
	resp := &profiler_service_pb.GenerateProfileResponse{}
	if profile != nil {
		resp.PerformanceProfile = profile
	} else {
		// This case implies profiler.GenerateProfile returned (nil, nil), which is unusual.
		// Or if profiler.GenerateProfile could return (nil, error) and the error was handled above.
		// If profile is nil and err was also nil, we might need to construct an empty profile
		// or return an internal error. For now, assume profile is valid if err is nil.
		resp.PerformanceProfile = &profiler_service_pb.PerformanceProfile{}
	}

	return resp, nil
}
