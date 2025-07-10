# Request Distribution Service (Go)

This directory contains the Request Distribution Generator gRPC service.

This service analyzes the characteristics of a set of Hugging Face datasets by calling the `process_dataset.py` script to determine different "Request Types" based on input/output token lengths. It then distributes a given total request rate across these types. This represents the request rate distribution if one sampled uniformly at random across all inputs/outputs in the dataset at the given rate.

The service exposes gRPC endpoints to:
1.  Update the reference datasets and total request rate, triggering a recalculation. (reflecting a change in distribution of incomming requests)
2.  Fetch the currently calculated request types and their rate distribution.

## Prerequisites

*   **Python 3:** Ensure Python 3 is installed and accessible as `python3`.
*   **Python Libraries:** Install the required Python libraries for the `process_dataset.py` script:
    ```bash
    pip install -r requirements.txt
    ```
*   **Hugging Face Login (Optional):** If you plan to use private or gated datasets, log in using the Hugging Face CLI:
    ```bash
    huggingface-cli login
    ```
    Alternatively, you can provide a Hugging Face token via a command-line flag when running the service.

## Building and Testing

This service is composed of a GO binary that calls a python script for HF dataset analysis.

**1. Generate Protobuf Code:**

The Go code for protobuf messages and gRPC service stubs is generated when building targets that depend on it. If you modify the `.proto` files, you may need to regenerate them explicitly:

```shell
protoc --proto_path=protos \
--go_out=. \
--go_opt=module=ig-wva \
--go-grpc_out=. \
--go-grpc_opt=module=ig-wva \
$(find protos -name "*.proto")
```

**2. Run Unit Tests:**

Unit tests for the service logic are located in `internal/server/server_test.go`. From the project root run (must be logged in with `huggingface-cli login` to access the datasets used in this test):

```shell
go test ./request_distribution_service/internal/server
```

**3. Build the Go Binary:**

Compile the gRPC server into a static binary (from project root directory):

```shell
go build -o bin/request_distribution_service ./request_distribution_service/cmd/server
```

This will place the executable at `bin` in project root.

## Running the Service

The service can be run locally using the provided `run_service.sh` script or by manually running the Go binary after setup.

### Using the `run_service.sh` script (Recommended for Local Testing):

The `run_service.sh` script automates building the Go binary, copying the necessary Python script (`process_dataset.py`) to the binary's location, making it executable, and starting the service with specified parameters.

1.  **Make the script executable:**

    ```bash
    chmod +x request_distribution_service/run_service.sh
    ```
2.  **Run from the project root directory:**

    **Example with a public dataset (e.g., `glue`):**

    ```bash
    ./request_distribution_service/run_service.sh \
      --initial_dataset_name "glue" \
      --initial_total_rate "100.0"
    ```
    The above example uses `glue` which is a smaller public dataset, good for quick testing. The `run_service.sh` script uses defaults for tokenizer (`bert-base-uncased`), input column (`text`), output column (`text`), and latency SLO (50ms). You can override these defaults by modifying the script or adding more arguments to it.

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
    cp request_distribution_service/process_dataset.py bin/process_dataset.py
    chmod +x bin/process_dataset.py
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

