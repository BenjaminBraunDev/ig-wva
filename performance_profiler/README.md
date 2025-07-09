# Performance Profile Generator Service

This directory contains the Go implementation of the Performance Profile Generator gRPC service.

This service takes a workload definition (WorkerTypes and RequestTypes) and generates a performance profile based on benchmarking data stored in Spanner or mock data. The profile indicates the maximum sustainable throughput (requests/second) for each (WorkerType, RequestType) combination, respecting latency SLOs.

## Building and Testing

This service uses Blaze for building and testing.

**1. (Optional) Generate Protobuf Code:**

The Go code for the protobuf messages and gRPC service is generated automatically when building other targets that depend on it (like the binary or tests). You can explicitly generate the message code using:

```shell
blaze build ig-wva/performance_profiler:profiler_service_go_proto
```

And the gRPC service code using:

```shell
blaze build ig-wva/performance_profiler:profiler_service_go_grpc
```

**2. Run Unit Tests:**

Unit tests for the core profiler logic and the gRPC server layer are available. Run them using:

```shell
blaze test ig-wva/performance_profiler/internal/profiler:profiler_test ig-wva/performance_profiler/internal/server:server_test
```

**3. Build the Go Binary:**

Compile the gRPC server into a static binary:

```shell
go build -o bin/performance_profiler ./performance_profiler/cmd/server
```

This will place the executable at `blaze-bin/experimental/users/benjaminbraun/gateway_autoscaling/performance_profiler/cmd/server/main`.

## Docker Image

A `Dockerfile` is provided to package the service binary into a minimal container image. (Assuming `Dockerfile` is in `experimental/users/benjaminbraun/gateway_autoscaling/performance_profiler/`)

**1. Build the Docker Image:**

Ensure you have built the Go binary first (see step 3 above). Then, from the `google3` root directory, run:

```shell
# Make sure the blaze binary exists first!
docker build -t profiler-service -f experimental/users/benjaminbraun/gateway_autoscaling/performance_profiler/Dockerfile .
```

*(Note: Adjust the `-t profiler-service` tag as needed. The build context `.` is the google3 root, allowing the `COPY` command in the Dockerfile to find the `blaze-bin` output, specifically `blaze-bin/experimental/users/benjaminbraun/gateway_autoscaling/performance_profiler/cmd/server/main`.)*

## Running the Service

**1. Running Locally (Directly):**

You can run the compiled binary directly, providing necessary flags:

To run with mock data (useful for local testing without Spanner):

```shell
# From google3 root:
./bin/performance_profiler \
  --port=8080 \
  --use_mock_data
```

To connect to Spanner:

```shell
# From google3 root:
./bin/performance_profiler \
  --port=8080 \
  --spanner_db="projects/YOUR_PROJECT/instances/YOUR_INSTANCE/databases/YOUR_DATABASE"
```

To run with data from a CSV file in GCS:

```shell
# From google3 root:
./bin/performance_profiler \
  --port=8080 \
  --gcs_csv_bucket="YOUR_GCS_BUCKET_NAME" \
  --gcs_csv_object="path/to/your/performance_data.csv"
```

For Spanner and GCS, replace placeholders with your actual Spanner database details.

**2. Running Locally (Docker):**

Run the container image built previously.

To connect to Spanner:

```shell
docker run -p 8080:8080 --rm \
  profiler-service \
  --port=8080 \
  --spanner_db="projects/YOUR_PROJECT/instances/YOUR_INSTANCE/databases/YOUR_DATABASE"
```

To run the Docker container with mock data:

```shell
docker run -p 8080:8080 --rm \
  profiler-service \
  --port=8080 \
  --use_mock_data
```

*(Note: When connecting to Spanner, this assumes the Docker host can authenticate. You might need to mount credentials or configure Application Default Credentials (ADC) for the Docker environment.)*

**3. Running on GKE:**

Deploy the container image to a GKE cluster. You will need to create Kubernetes deployment and service manifests. Ensure the GKE nodes/pods have appropriate service accounts and permissions to access the specified Spanner database if not using mock data. Pass the necessary flags (e.g., `--spanner_db` or `--use_mock_data`) via the deployment's `args`.

## Authentication

When connecting to Spanner, the service uses the standard Google Cloud Go client libraries, which typically rely on Application Default Credentials (ADC) for authentication. Ensure the environment where the service runs (local machine, Docker container, GKE pod) has valid credentials configured if you are not using the `--use_mock_data` flag.

## Verification

Once the server is running with mock data (either directly or via Docker), you can use `grpcurl` to send requests and verify its functionality. If you don't have `grpcurl`, you can usually install it via your system's package manager (e.g., `sudo apt-get install grpcurl` on Debian/Ubuntu, `brew install grpcurl` on macOS).

**1. Start the Server with Mock Data:**

Ensure the server is running. For example, directly:

```shell
# From google3 root:
./blaze-bin/experimental/users/benjaminbraun/gateway_autoscaling/performance_profiler/cmd/server/main --port=8080 --use_mock_data
```
Or using Docker:

```shell
docker run -p 8080:8080 --rm profiler-service --port=8080 --use_mock_data
```
The server should output: `Server listening at [::]:8080`.

**2. Check Health Status:**

In a new terminal, check the health of the `PerformanceProfileGenerator` service:

```shell
grpcurl -plaintext -d '{"service": "profiler_service.PerformanceProfileGenerator"}' localhost:8080 grpc.health.v1.Health/Check
```
Expected Output:

```json
{
  "status": "SERVING"
}
```

**3. Test `GenerateProfile` with Mock Data:**

Send a request with the three mock request types defined in `cmd/server/main.go`:

```shell
grpcurl -plaintext -d '{
  "workload_definition": {
    "worker_types": [
      {
        "id": "worker_mock_01",
        "model_name": "mock_model_gemma",
        "accelerator_type": "mock_accel_l4",
        "accelerator_count": 1,
        "model_server_type": "TGI",
        "model_server_image": "tgi_image_v1"
      }
    ],
    "request_types": [
      {
        "id": "request_mock_01",
        "latency_slo_tpot_ms": 250.0,
        "input_size_bucket": "S",
        "output_size_bucket": "S"
      },
      {
        "id": "request_mock_02_slo_unattainable",
        "latency_slo_tpot_ms": 50.0,
        "input_size_bucket": "M",
        "output_size_bucket": "M"
      },
      {
        "id": "request_mock_03_ok_highest_rate",
        "latency_slo_tpot_ms": 150.0,
        "input_size_bucket": "L",
        "output_size_bucket": "L"
      }
    ]
  }
}' \
localhost:8080 \
experimental.users.benjaminbraun.gateway_autoscaling.performance_profiler.PerformanceProfileGenerator/GenerateProfile
```
Expected Output:

```json
{
  "performanceProfile": {
    "entries": [
      {
        "workerTypeId": "worker_mock_01",
        "requestTypeId": "request_mock_01",
        "maxThroughputRps": 23.333334,
        "status": "OK"
      },
      {
        "workerTypeId": "worker_mock_01",
        "requestTypeId": "request_mock_02_slo_unattainable",
        "maxThroughputRps": 0,
        "status": "SLO_UNATTAINABLE"
      },
      {
        "workerTypeId": "worker_mock_01",
        "requestTypeId": "request_mock_03_ok_highest_rate",
        "maxThroughputRps": 25,
        "status": "OK_USING_HIGHEST_RATE"
      }
    ]
  }
}
```
*(Note: The order of entries in the `performanceProfile` array might vary. The `maxThroughputRps` for the first entry is an approximation and might have slight floating-point variations.)*

## Verification With GCS Bucket Data

Run with data from a CSV file in GCS:

```shell
# From google3 root:
./blaze-bin/experimental/users/benjaminbraun/gateway_autoscaling/performance_profiler/cmd/server/main \
  --port=8080 \
  --gcs_csv_bucket="YOUR_GCS_BUCKET_NAME" \
  --gcs_csv_object="path/to/your/performance_data.csv"
```

From there you can generate a performance profile using that data. Here is an example workload distribution:

```shell
grpcurl -plaintext -d '{
  "workload_definition": {
  "worker_types": [
    {
      "id": "h100_80gb_worker_config1",
      "model_name": "some_model_name_h100",
      "accelerator_type": "NVIDIA_H100_80GB",
      "accelerator_count": 1,
      "model_server_type": "some_server_type",
      "model_server_image": "some_server_image_tag_h100"
    },
    {
      "id": "l4_worker_config1",
      "model_name": "some_model_name_l4",
      "accelerator_type": "NVIDIA_L4",
      "accelerator_count": 1,
      "model_server_type": "some_server_type",
      "model_server_image": "some_server_image_tag_l4"
    }
  ],
  "request_types": [
    {
      "id": "req_in_2_3_out_2_3_slo_10",
      "latency_slo_tpot_ms": 10.0,
      "input_size_bucket": "2-3",
      "output_size_bucket": "2-3"
    },
    {
      "id": "req_in_2_3_out_2_3_slo_100",
      "latency_slo_tpot_ms": 100.0,
      "input_size_bucket": "2-3",
      "output_size_bucket": "2-3"
    },
    {
      "id": "req_in_2_3_out_32_63_slo_10",
      "latency_slo_tpot_ms": 10.0,
      "input_size_bucket": "2-3",
      "output_size_bucket": "32-63"
    },
    {
      "id": "req_in_2_3_out_32_63_slo_50",
      "latency_slo_tpot_ms": 100.0,
      "input_size_bucket": "2-3",
      "output_size_bucket": "32-63"
    },
    {
      "id": "req_in_2_3_out_32_63_slo_250",
      "latency_slo_tpot_ms": 250.0,
      "input_size_bucket": "2-3",
      "output_size_bucket": "32-63"
    },
    {
      "id": "req_in_512_1023_out_512_1023_slo_20",
      "latency_slo_tpot_ms": 20.0,
      "input_size_bucket": "512-1023",
      "output_size_bucket": "512-1023"
    },
    {
      "id": "req_in_4096_8191_out_512_1023_slo_20",
      "latency_slo_tpot_ms": 20.0,
      "input_size_bucket": "4096-8191",
      "output_size_bucket": "512-1023"
    },
    {
      "id": "req_in_4096_8191_out_512_1023_slo_250",
      "latency_slo_tpot_ms": 250.0,
      "input_size_bucket": "4096-8191",
      "output_size_bucket": "512-1023"
    }
  ]
}
}' \
localhost:8080 \
experimental.users.benjaminbraun.gateway_autoscaling.performance_profiler.PerformanceProfileGenerator/GenerateProfile
```

Output will vary based on the benchmarking data used.