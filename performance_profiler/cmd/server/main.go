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

	"ig-wva/performance_profiler/internal/datasource"
	"ig-wva/performance_profiler/internal/profiler"
	"ig-wva/performance_profiler/internal/server"

	benchmarking_data_pb "ig-wva/gen/go/benchmarking_data"
	profiler_service_grpc "ig-wva/gen/go/profiler_service"
)

var (
	// Define flags for configuration
	port         = flag.Int("port", 8080, "The server port")
	useMockData  = flag.Bool("use_mock_data", false, "Use mock data source instead of a real one")
	gcsCsvBucket = flag.String("gcs_csv_bucket", "", "GCS bucket name for CSV data source")
	gcsCsvObject = flag.String("gcs_csv_object", "", "GCS object path for CSV data source (e.g., path/to/data.csv)")
	// Add the new flag for the local CSV file path
	csvFilePath = flag.String("csv_file_path", "", "Path to a local CSV file data source.")
)

func main() {
	// Parse flags
	flag.Parse()
	// log.Init() // Initialize Google logging if used

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var ds datasource.BenchmarkingDataSource // Use the interface type

	if *useMockData {
		fmt.Println("Using mock data source.")
		// log.Info(ctx, "Using mock data source.")
		mockDS := profiler.NewMockBenchmarkingDataSource()

		// Define sample worker and request types for mock data
		mockWorkerID := "worker_mock_01"
		mockRequestID := "request_mock_01"
		mockRequestID2 := "request_mock_02_slo_unattainable"
		mockRequestID3 := "request_mock_03_ok_highest_rate"

		// Populate data for worker_mock_01 and request_mock_01
		sampleDataPoints1 := []*benchmarking_data_pb.BenchmarkingDataPoint{
			{
				MeasuredRequestRateRps: 10.0,
				MeasuredLatencyTpotMs:  100.0,
				AcceleratorType:        "mock_accel_l4",
				AcceleratorCount:       1,
				ModelName:              "mock_model_gemma",
				InputSizeBucket:        "S",
				OutputSizeBucket:       "S",
				ModelServerType:        "TGI",
				ModelServerImage:       "tgi_image_v1",
			},
			{
				MeasuredRequestRateRps: 20.0,
				MeasuredLatencyTpotMs:  200.0,
				AcceleratorType:        "mock_accel_l4",
				AcceleratorCount:       1,
				ModelName:              "mock_model_gemma",
				InputSizeBucket:        "S",
				OutputSizeBucket:       "S",
				ModelServerType:        "TGI",
				ModelServerImage:       "tgi_image_v1",
			},
			{
				MeasuredRequestRateRps: 30.0,
				MeasuredLatencyTpotMs:  350.0,
				AcceleratorType:        "mock_accel_l4",
				AcceleratorCount:       1,
				ModelName:              "mock_model_gemma",
				InputSizeBucket:        "S",
				OutputSizeBucket:       "S",
				ModelServerType:        "TGI",
				ModelServerImage:       "tgi_image_v1",
			},
		}
		mockDS.SetDataPoints(mockWorkerID, mockRequestID, sampleDataPoints1)

		// Populate data for worker_mock_01 and request_mock_02 (SLO Unattainable case)
		sampleDataPoints2 := []*benchmarking_data_pb.BenchmarkingDataPoint{
			{
				MeasuredRequestRateRps: 5.0,
				MeasuredLatencyTpotMs:  150.0, // Lowest latency is 150ms
				AcceleratorType:        "mock_accel_l4",
				AcceleratorCount:       1,
				ModelName:              "mock_model_gemma",
				InputSizeBucket:        "M",
				OutputSizeBucket:       "M",
				ModelServerType:        "TGI",
				ModelServerImage:       "tgi_image_v1",
			},
		}
		mockDS.SetDataPoints(mockWorkerID, mockRequestID2, sampleDataPoints2)

		// Populate data for worker_mock_01 and request_mock_03 (OK_USING_HIGHEST_RATE case)
		sampleDataPoints3 := []*benchmarking_data_pb.BenchmarkingDataPoint{
			{
				MeasuredRequestRateRps: 15.0,
				MeasuredLatencyTpotMs:  80.0, // Lower rate, lower latency
				AcceleratorType:        "mock_accel_l4",
				AcceleratorCount:       1,
				ModelName:              "mock_model_gemma",
				InputSizeBucket:        "L", // Different bucket for variety
				OutputSizeBucket:       "L",
				ModelServerType:        "TGI",
				ModelServerImage:       "tgi_image_v1",
			},
			{
				MeasuredRequestRateRps: 25.0,
				MeasuredLatencyTpotMs:  120.0, // Higher rate, higher latency
				AcceleratorType:        "mock_accel_l4",
				AcceleratorCount:       1,
				ModelName:              "mock_model_gemma",
				InputSizeBucket:        "L",
				OutputSizeBucket:       "L",
				ModelServerType:        "TGI",
				ModelServerImage:       "tgi_image_v1",
			},
		}
		mockDS.SetDataPoints(mockWorkerID, mockRequestID3, sampleDataPoints3)

		ds = mockDS
	} else if *csvFilePath != "" { // Check for the new local file flag
		fmt.Printf("Using local CSV file data source: %s\n", *csvFilePath)
		fileDS, err := datasource.NewFileDataSource(*csvFilePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating local file data source: %v\n", err)
			os.Exit(1)
		}
		ds = fileDS
	} else if *gcsCsvBucket != "" && *gcsCsvObject != "" {
		fmt.Printf("Using GCS CSV data source: gs://%s/%s\n", *gcsCsvBucket, *gcsCsvObject)
		// log.Infof(ctx, "Using GCS CSV data source: gs://%s/%s", *gcsCsvBucket, *gcsCsvObject)
		gcsDS, err := datasource.NewGCSDataSource(ctx, *gcsCsvBucket, *gcsCsvObject)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating GCS data source: %v\n", err)
			// log.Fatalf(ctx, "Error creating GCS data source: %v", err)
			os.Exit(1)
		}
		ds = gcsDS
	} else {
		// --- Validate Configuration ---
		// Update the error message to include the new flag
		fmt.Fprintln(os.Stderr, "Error: A data source must be specified. Use --use_mock_data, --csv_file_path, or GCS flags (--gcs_csv_bucket and --gcs_csv_object).")
		// log.Fatal(ctx, "Error: A data source must be specified.")
		os.Exit(1)
	}

	// --- Create Service Components ---
	coreProfiler := profiler.NewProfiler(ds) // Pass the chosen datasource
	grpcServerImpl := server.NewServer(coreProfiler)

	// --- Setup gRPC Server ---
	grpcServer := grpc.NewServer(
	// Add interceptors if needed (e.g., for auth, logging, metrics)
	)
	profiler_service_grpc.RegisterPerformanceProfileGeneratorServer(grpcServer, grpcServerImpl)

	// Optional: Register reflection service for tools like grpcurl
	reflection.Register(grpcServer)

	// Optional: Register health checking service
	healthServer := health.NewServer()
	healthgrpc.RegisterHealthServer(grpcServer, healthServer)

	serviceName := profiler_service_grpc.PerformanceProfileGenerator_ServiceDesc.ServiceName
	healthServer.SetServingStatus(serviceName, healthgrpc.HealthCheckResponse_SERVING)

	// --- Start Listening ---
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen: %v\n", err)
		// log.Fatalf(ctx, "Failed to listen: %v", err)
		os.Exit(1)
	}
	fmt.Printf("Server listening at %v\n", lis.Addr())
	// log.Infof(ctx, "Server listening at %v", lis.Addr())

	// --- Start gRPC Server in Goroutine ---
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to serve gRPC: %v\n", err)
			// log.Fatalf(ctx, "Failed to serve gRPC: %v", err)
			cancel() // Signal main goroutine to exit
		}
	}()

	// --- Graceful Shutdown Handling ---
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stopChan:
		fmt.Printf("Received signal %v, shutting down gracefully...\n", sig)
		// log.Infof(ctx, "Received signal %v, shutting down gracefully...", sig)
	case <-ctx.Done(): // If context was cancelled due to server error
		fmt.Println("Context cancelled, shutting down...")
		// log.Infof(ctx, "Context cancelled, shutting down...")
	}

	// Set health status to NOT_SERVING for the specific service
	healthServer.SetServingStatus(serviceName, healthgrpc.HealthCheckResponse_NOT_SERVING)

	// Perform graceful shutdown
	grpcServer.GracefulStop()
	fmt.Println("gRPC server stopped.")
	// log.Info(ctx, "gRPC server stopped.")
}
