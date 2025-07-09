// Package main implements the gRPC server for the RequestDistributionGenerator service.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"

	rdpb "ig-wva/gen/go/request_distribution"
	serverinternal "ig-wva/request_distribution_service/internal/server"
)

var (
	port = flag.Int("port", 8081, "The server port for the request distribution service.")
	// Add initial dataset args if needed, or rely on UpdateDatasetAndRates RPC
	initialDatasetRequestsJSON = flag.String("initial_dataset_requests_json", "", "A JSON string representing an array of initial dataset requests to load on startup.")
	initialHfToken             = flag.String("initial_hf_token", "", "Initial Hugging Face API token.")
)

func main() {
	flag.Parse()
	// log.Init() // Initialize Google logging if used

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize the core logic handler
	coreLogic := serverinternal.NewCoreLogicHandler()
	coreLogic.SetHfToken(*initialHfToken)

	// If initial dataset parameters are provided, load them on startup.
	if *initialDatasetRequestsJSON != "" {
		fmt.Printf("Attempting to load initial dataset from JSON: %s\n", *initialDatasetRequestsJSON)
		var datasetRequests []*rdpb.DatasetRequest
		unmarshalOptions := protojson.UnmarshalOptions{DiscardUnknown: true}
		var datasetRequestsWrapper rdpb.UpdateDatasetAndRatesRequest
		if err := unmarshalOptions.Unmarshal([]byte(*initialDatasetRequestsJSON), &datasetRequestsWrapper); err != nil {
			fmt.Fprintf(os.Stderr, "Error unmarshalling initial_dataset_requests_json: %v\n", err)
			os.Exit(1)
		}
		datasetRequests = datasetRequestsWrapper.GetDatasetRequests()

		updateReq := &rdpb.UpdateDatasetAndRatesRequest{DatasetRequests: datasetRequests}

		_, err := coreLogic.UpdateDatasetAndRates(ctx, updateReq)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading initial dataset: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Initial dataset processed.")
	}

	// --- Setup gRPC Server ---
	grpcServer := grpc.NewServer()
	rdpb.RegisterRequestDistributionGeneratorServer(grpcServer, coreLogic) // Register coreLogic as it implements the service

	reflection.Register(grpcServer)

	healthServer := health.NewServer()
	healthgrpc.RegisterHealthServer(grpcServer, healthServer)
	serviceName := rdpb.RequestDistributionGenerator_ServiceDesc.ServiceName
	fmt.Printf("Registering health status for service: %s\n", serviceName)
	healthServer.SetServingStatus(serviceName, healthgrpc.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen: %v\n", err)
		// log.Fatalf(ctx, "Failed to listen: %v", err)
		os.Exit(1)
	}
	fmt.Printf("Request Distribution Generator server listening at %v\n", lis.Addr())
	// log.Infof(ctx, "Request Distribution Generator server listening at %v", lis.Addr())

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to serve gRPC: %v\n", err)
			// log.Fatalf(ctx, "Failed to serve gRPC: %v", err)
			cancel()
		}
	}()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stopChan:
		fmt.Printf("Received signal %v, shutting down gracefully...\n", sig)
		// log.Infof(ctx, "Received signal %v, shutting down gracefully...", sig)
	case <-ctx.Done():
		fmt.Println("Context cancelled, shutting down...")
		// log.Info(ctx, "Context cancelled, shutting down...")
	}

	healthServer.SetServingStatus(string(serviceName), healthgrpc.HealthCheckResponse_NOT_SERVING)
	grpcServer.GracefulStop()
	fmt.Println("gRPC server stopped.")
	// log.Info(ctx, "gRPC server stopped.")
}
