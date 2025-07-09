# Request Distribution Service (Go)

This directory contains the Go implementation of the Request Distribution Generator gRPC service.

This service analyzes Hugging Face dataset characteristics by calling an internal Python script (`process_dataset.py`) to determine different "Request Types" based on input/output token lengths. It then distributes a given total request rate across these types.

The service exposes gRPC endpoints to:
1.  Update the reference dataset and total request rate, triggering a recalculation.
2.  Fetch the currently calculated request types and their rate distribution.

## Prerequisites

*   **Python 3:** Ensure Python 3 is installed and accessible as `python3`.
*   **Python Libraries:** Install the required Python libraries for the `process_dataset.py` script:
    ```bash
    pip install datasets tqdm transformers huggingface_hub
    ```
*   **Hugging Face Login (Optional):** If you plan to use private or gated datasets, log in using the Hugging Face CLI:
    ```bash
    huggingface-cli login
    ```
    Alternatively, you can provide a Hugging Face token via a command-line flag when running the service.

## Building and Testing

This service uses Blaze for building and testing.

**1. Generate Protobuf Code (if modified):**

The Go code for protobuf messages and gRPC service stubs is generated when building targets that depend on it. If you modify the `.proto` files, you may need to regenerate them explicitly:

```shell
blaze build //experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/proto:request_distribution_go_proto
blaze build //experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/proto:request_distribution_go_grpc
```

**2. Run Unit Tests:**

Unit tests for the service logic are located in `internal/server/server_test.go`.

```shell
blaze test //experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/internal/server:server_test
```

**3. Build the Go Binary:**

Compile the gRPC server into a static binary:

```shell
go build -o bin/request_distribution_service ./request_distribution_service/cmd/server
```

This will place the executable at `blaze-bin/experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/cmd/server/server`.

## Running the Service

The service can be run locally using the provided `run_service.sh` script or by manually running the Go binary after setup.

### Using the `run_service.sh` script (Recommended for Local Testing):

The `run_service.sh` script automates building the Go binary, copying the necessary Python script (`process_dataset.py`) to the binary's location, making it executable, and starting the service with specified parameters.

1.  **Make the script executable:**

    ```bash
    chmod +x experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/run_service.sh
    ```
2.  **Run from the `google3` root directory:**

    **Example with a public dataset (e.g., `glue`):**

    ```bash
    ./request_distribution_service/run_service.sh \
      --initial_dataset_name "glue" \
      --initial_total_rate "100.0"
    ```
    This example uses `glue` which is a smaller public dataset, good for quick testing. The `run_service.sh` script uses defaults for tokenizer (`bert-base-uncased`), input column (`text`), output column (`text`), and latency SLO (50ms). You can override these defaults by modifying the script or adding more arguments to it.

    **To use a specific Hugging Face token (e.g., for private datasets):**
    
    ```bash
    ./request_distribution_service/run_service.sh \
      --initial_dataset_name "your_private_dataset_name" \
      --initial_total_rate "100.0" \
      --hf_token "YOUR_HUGGING_FACE_TOKEN"
    ```

### Running Manually (Directly):

1.  **Build the Go binary** (as shown in the "Building and Testing" section).
2.  **Copy the Python script:** The Go service expects `process_dataset.py` to be in the same directory as the Go binary.

    ```bash
    # From project root
    cp request_distribution_service/process_dataset.py blaze-bin/experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/cmd/server/
    chmod +x request_distribution_service/cmd/server/process_dataset.py
    ```
3.  **Navigate to the binary's directory and run:**

    ```bash
    # From project root
    cd request_distribution_service/cmd/server/
    ./server \
      --port=8081 \
      --initial_dataset_name="glue" \
      --initial_dataset_config="mrpc" \
      --initial_tokenizer_name="bert-base-uncased" \
      --initial_input_column="sentence1" \
      --initial_output_column="sentence2" \
      --initial_total_request_rate=100.0 \
      --initial_latency_slo_tpot_ms=50.0 \
      # --initial_hf_token="YOUR_HUGGING_FACE_TOKEN" # Optional
    ```

## Docker Image

A `Dockerfile` is provided to package the service binary into a minimal container image.

1.  **Build the Go binary and the Python script first.** Ensure `process_dataset.py` is executable.
2.  **Build the Docker Image (from `google3` root directory):**
    The Dockerfile expects the Go binary and the Python script to be in the `blaze-bin` output directory of the Go service.
    You'll need to ensure `process_dataset.py` is copied to `blaze-bin/experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/cmd/server/` before building the Docker image if it's not already handled by your build process.

    ```bash
    # First, ensure the python script is where the Dockerfile expects it:
    # (Assuming you are in google3 root)
    mkdir -p blaze-bin/experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/cmd/server/
    cp experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/process_dataset.py blaze-bin/experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/cmd/server/
    chmod +x blaze-bin/experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/cmd/server/process_dataset.py

    # Then build the image
    docker build -t request-distribution-service -f experimental/users/benjaminbraun/gateway_autoscaling/request_distribution_service_go/Dockerfile .
    ```
    The Dockerfile needs to be updated to copy `process_dataset.py` and ensure `python3` and dependencies are available in the image.

## Verification (using grpcurl)

**1. Start the Server:**

Ensure the server is running (e.g., locally using `run_service.sh` or manually).

**2. Check Health Status:**

```shell
grpcurl -plaintext -d '{"service": "request_distribution.RequestDistributionGenerator"}' localhost:8081 grpc.health.v1.Health/Check
```
Expected Output:

```json
{
  "status": "SERVING"
}
```

**3. Update Dataset and Rates Examples:**

These examples show how to change the dataset the service is analyzing. You can call `GetCurrentDistribution` after each update to see the new request types and rates.

**Example A: Updating with a single dataset:**

```shell
grpcurl -plaintext -d '{
  "dataset_requests": [
    {
      "dataset_name": "squad",
      "dataset_config": "plain_text",
      "tokenizer_name": "bert-base-cased",
      "input_column": "question",
      "output_column": "context",
      "total_request_rate": 150.0,
      "latency_slo_tpot_ms": 75.0,
      "max_samples": 100
    }
  ]
}' \
localhost:8081 request_distribution.RequestDistributionGenerator/UpdateDatasetAndRates
```

**Example B: Updating with multiple datasets for a cumulative distribution:**

```shell
grpcurl -plaintext -d '{
  "dataset_requests": [
    {
      "dataset_name": "glue",
      "dataset_config": "mrpc",
      "tokenizer_name": "bert-base-uncased",
      "input_column": "sentence1",
      "output_column": "sentence2",
      "total_request_rate": 100.0,
      "latency_slo_tpot_ms": 50.0
    },
    {
      "dataset_name": "squad",
      "dataset_config": "plain_text",
      "tokenizer_name": "bert-base-cased",
      "input_column": "question",
      "output_column": "context",
      "total_request_rate": 50.0,
      "latency_slo_tpot_ms": 75.0
    }
  ]
}' \
localhost:8081 \
request_distribution.RequestDistributionGenerator/UpdateDatasetAndRates
```

Expected Output:

```json
{
  "message": "Dataset and rates updated successfully"
}
```

**4. Get Current Distribution:**

```shell
grpcurl -plaintext localhost:8081 request_distribution.RequestDistributionGenerator/GetCurrentDistribution
```
Output will vary based on the dataset and parameters used.

