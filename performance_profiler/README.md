# Performance Profile Generator Service

This directory contains the Performance Profile Generator gRPC service.

This service takes a workload definition (WorkerTypes and RequestTypes) and generates a performance profile based on benchmarking data stored in a GCS bucket or mock data. The profile indicates the maximum sustainable throughput (requests/second) for each (WorkerType, RequestType) combination, respecting latency SLOs.

## Building and Testing

**1. Generate Protobuf Code:**

The Go code for the protobuf messages and gRPC service is generated to the `gen/go` directory in the root of the project:

```shell
protoc --proto_path=protos \
--go_out=. \
--go_opt=module=ig-wva \
--go-grpc_out=. \
--go-grpc_opt=module=ig-wva \
$(find protos -name "*.proto")
```

**2. Run Unit Tests:**

Unit tests for the core profiler logic and the gRPC server layer are available. Run them using:

```shell
go test ./performance_profiler/internal/profiler
go test ./performance_profiler/internal/server
```

**3. Build the Go Binary:**

Compile the gRPC server into a static binary:

```shell
go build -o bin/performance_profiler ./performance_profiler/cmd/server
```

This will place the executable at `bin` in project root.

## Running the Service

**1. Running Locally (Directly):**

You can run the compiled binary directly, providing necessary flags:

To run with mock data (useful for local testing without Spanner):

```shell
./bin/performance_profiler \
  --port=8080 \
  --use_mock_data
```

To run with data from a CSV file in GCS:

```shell
./bin/performance_profiler \
  --port=8080 \
  --gcs_csv_bucket="YOUR_GCS_BUCKET_NAME" \
  --gcs_csv_object="path/to/your/performance_data.csv"
```

## Verification With Mock Data

Once the server is running, you can use `grpcurl` to send requests and verify its functionality. If you don't have `grpcurl`, you can usually install it via your system's package manager (e.g., `sudo apt-get install grpcurl` on Debian/Ubuntu, `brew install grpcurl` on macOS).

**1. Start the Server with Mock Data:**

Ensure the server is running. For example, directly:

```shell
./bin/performance_profiler --port=8080 --use_mock_data
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
profiler_service.PerformanceProfileGenerator/GenerateProfile
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
./bin/performance_profiler \
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
profiler_service.PerformanceProfileGenerator/GenerateProfile
```

Output will vary based on the benchmarking data used.