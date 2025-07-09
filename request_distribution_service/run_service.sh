# Script to build and run the Request Distribution Go service.

# Default values - can be overridden by command-line arguments
# or modified here if needed.
DEFAULT_TOKENIZER_NAME="bert-base-uncased"
DEFAULT_INPUT_COLUMN="text"
DEFAULT_OUTPUT_COLUMN="text"
DEFAULT_LATENCY_SLO_MS="50.0"
DEFAULT_PORT="8081"
DEFAULT_DATASET_CONFIG="mrpc" # or None, depending on dataset
DEFAULT_MAX_SAMPLES="100"

# --- Argument Parsing ---
HF_TOKEN=""
INITIAL_TOTAL_RATE=""
INITIAL_DATASET_CONFIG=""
INITIAL_DATASET_NAME=""
INITIAL_INPUT_COLUMN=""
INITIAL_OUTPUT_COLUMN=""

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --hf_token) HF_TOKEN="$2"; shift ;;
        --initial_total_rate) INITIAL_TOTAL_RATE="$2"; shift ;;
        --initial_dataset_config) INITIAL_DATASET_CONFIG="$2"; shift ;;
        --initial_dataset_name) INITIAL_DATASET_NAME="$2"; shift ;;
        --initial_input_column) INITIAL_INPUT_COLUMN="$2"; shift ;;
        --initial_output_column) INITIAL_OUTPUT_COLUMN="$2"; shift ;;
        *) echo "Unknown parameter passed: $1"; exit 1 ;;
    esac
    shift
done

if [ -z "$INITIAL_TOTAL_RATE" ] || [ -z "$INITIAL_DATASET_NAME" ]; then
    echo "Usage: $0 --initial_dataset_name <dataset_name> --initial_total_rate <rate> [--hf_token <token>] [--initial_dataset_config <dataset_config>] [--initial_input_column <input_column>] [--initial_output_column <output_column>]"
    exit 1
fi

if [ -z "$INITIAL_DATASET_CONFIG" ]; then
  INITIAL_DATASET_CONFIG="$DEFAULT_DATASET_CONFIG"
fi

# Set FINAL_INPUT_COLUMN and FINAL_OUTPUT_COLUMN
if [ "$INITIAL_DATASET_NAME" == "glue" ] && [ "$INITIAL_DATASET_CONFIG" == "mrpc" ]; then
  if [ -z "$INITIAL_INPUT_COLUMN" ]; then
    FINAL_INPUT_COLUMN="sentence1"
  else
    FINAL_INPUT_COLUMN="$INITIAL_INPUT_COLUMN"
  fi
  if [ -z "$INITIAL_OUTPUT_COLUMN" ]; then
    FINAL_OUTPUT_COLUMN="sentence2"
  else
    FINAL_OUTPUT_COLUMN="$INITIAL_OUTPUT_COLUMN"
  fi
else
  if [ -z "$INITIAL_INPUT_COLUMN" ]; then
    FINAL_INPUT_COLUMN="$DEFAULT_INPUT_COLUMN"
  else
    FINAL_INPUT_COLUMN="$INITIAL_INPUT_COLUMN"
  fi
  if [ -z "$INITIAL_OUTPUT_COLUMN" ]; then
    FINAL_OUTPUT_COLUMN="$DEFAULT_OUTPUT_COLUMN"
  else
    FINAL_OUTPUT_COLUMN="$INITIAL_OUTPUT_COLUMN"
  fi
fi

# --- 1. Build the Go Binary ---
echo "Building Go service binary..."
# Assuming this script is run from the project root.
go build -o bin/request_distribution_service ./request_distribution_service/cmd/server
if [ $? -ne 0 ]; then
    echo "Blaze build failed. Exiting."
    exit 1
fi
echo "Go binary built successfully."

# Determine paths
# Adjust these paths if the script is located elsewhere or binary location is different.
SERVICE_BINARY_PATH="bin/request_distribution_service"
PYTHON_SCRIPT_SOURCE_PATH="request_distribution_service/process_dataset.py"
BINARY_DIR=$(dirname "$SERVICE_BINARY_PATH")
PYTHON_SCRIPT_DEST_PATH="$BINARY_DIR/process_dataset.py"

if [ ! -f "$SERVICE_BINARY_PATH" ]; then
    echo "Error: Service binary not found at $SERVICE_BINARY_PATH"
    exit 1
fi

if [ ! -f "$PYTHON_SCRIPT_SOURCE_PATH" ]; then
    echo "Error: Python script not found at $PYTHON_SCRIPT_SOURCE_PATH"
    exit 1
fi

# --- 2. Copy Python script to binary directory ---
echo "Copying Python script to $PYTHON_SCRIPT_DEST_PATH..."
cp "$PYTHON_SCRIPT_SOURCE_PATH" "$PYTHON_SCRIPT_DEST_PATH"
if [ $? -ne 0 ]; then
    echo "Failed to copy Python script. Exiting."
    exit 1
fi

# --- 3. Make Python script executable ---
echo "Making Python script executable..."
chmod +x "$PYTHON_SCRIPT_DEST_PATH"
if [ $? -ne 0 ]; then
    echo "Failed to make Python script executable. Exiting."
    exit 1
fi
echo "Python script setup complete."

# --- 4. Run the Go Service ---
echo "Starting Go service..."
# Construct the JSON for the initial dataset request
INITIAL_DATASET_JSON=$(printf '{"dataset_requests": [ {
  "dataset_name": "%s",
  "tokenizer_name": "%s",
  "input_column": "%s",
  "output_column": "%s",
  "dataset_config": "%s",
  "total_request_rate": %s,
  "latency_slo_tpot_ms": %s,
  "max_samples": %s
  } ] }' \
"$INITIAL_DATASET_NAME" \
"$DEFAULT_TOKENIZER_NAME" \
"$FINAL_INPUT_COLUMN" \
"$FINAL_OUTPUT_COLUMN" \
"$INITIAL_DATASET_CONFIG" \
"$INITIAL_TOTAL_RATE" \
"$DEFAULT_LATENCY_SLO_MS" \
"$DEFAULT_MAX_SAMPLES")

echo "Initial Dataset Request JSON: $INITIAL_DATASET_JSON"

# Navigate to the binary directory to ensure the Go service can find the Python script via relative path
cd "$BINARY_DIR" || exit

./$(basename "$SERVICE_BINARY_PATH") \
  --port="$DEFAULT_PORT" \
  --initial_dataset_requests_json="$INITIAL_DATASET_JSON" \
  ${HF_TOKEN:+--initial_hf_token="$HF_TOKEN"}

# The service will run in the foreground. Ctrl+C to stop.
echo "Service startup command executed. Service is running."
echo "To stop the service, press Ctrl+C."

wait
echo "Service stopped."
