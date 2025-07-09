#!/usr/bin/env python3
import argparse
from collections import Counter
import json
import math
from datasets import load_dataset
from tqdm import tqdm
from transformers import AutoTokenizer


def _get_power_of_two_bucket(n):
  """(Internal helper) Calculates the power-of-two bucket string."""
  if n <= 1:
    return "0-1"
  k = math.floor(math.log2(n))
  lower_bound = 2**k
  upper_bound = 2 ** (k + 1) - 1
  return f"{lower_bound}-{upper_bound}"


def generate_config(args):
  """Generates request types and rate distribution from dataset args."""
  print(f"Loading tokenizer: {args.tokenizer_name}...")
  tokenizer = AutoTokenizer.from_pretrained(
      args.tokenizer_name, token=args.hf_token
  )

  print(
      f"Loading dataset: {args.dataset_name} (config: {args.dataset_config})..."
  )
  # Use 'default' if None is provided for dataset_config, as load_dataset handles it better.
  dataset_config_to_load = (
      args.dataset_config if args.dataset_config else "default"
  )
  dataset = load_dataset(
      args.dataset_name,
      name=dataset_config_to_load,
      token=args.hf_token,
      trust_remote_code=True,
  )

  # Try to get 'train' split, otherwise the first available split
  if "train" in dataset:
    dataset_slice = dataset["train"]
  else:
    try:
      first_split = next(iter(dataset))
      dataset_slice = dataset[first_split]
      print(
          "Warning: 'train' split not found. Using the first available split:"
          f" '{first_split}'"
      )
    except StopIteration:
      raise ValueError(f"No splits found in dataset {args.dataset_name}")

  if args.max_samples and args.max_samples < len(dataset_slice):
    dataset_slice = dataset_slice.select(range(args.max_samples))

  all_token_counts = []
  print("Analyzing dataset and counting tokens...")
  for example in tqdm(dataset_slice, desc="Processing samples"):
    input_text = None
    output_text = None

    if (
        "conversations" in example
        and isinstance(example["conversations"], list)
        and len(example["conversations"]) >= 2
    ):
      input_text = example["conversations"][0].get("value", "")
      output_text = example["conversations"][1].get("value", "")
    elif (
        "messages" in example
        and isinstance(example["messages"], list)
        and len(example["messages"]) >= 2
    ):
      input_text = example["messages"][0].get("content", "")
      output_text = example["messages"][1].get("content", "")
    else:
      input_text = example.get(args.input_column, "")
      output_text = example.get(args.output_column, "")

    if not input_text or not output_text:
      continue

    if args.input_text_prefix:
      input_text = args.input_text_prefix + input_text

    input_tokens = tokenizer(input_text, truncation=False, padding=False)[
        "input_ids"
    ]
    output_tokens = tokenizer(output_text, truncation=False, padding=False)[
        "input_ids"
    ]
    all_token_counts.append((len(input_tokens), len(output_tokens)))

  if not all_token_counts:
    print("Warning: No valid prompt-response pairs found in the dataset.")
    return {}

  print("Generating request type and rate distribution...")
  bucket_counts = Counter(
      (_get_power_of_two_bucket(in_c), _get_power_of_two_bucket(out_c))
      for in_c, out_c in all_token_counts
  )

  sorted_keys = sorted(
      bucket_counts.keys(),
      key=lambda x: (int(x[0].split("-")[0]), int(x[1].split("-")[0])),
  )

  request_types = []
  rate_distribution = []
  total_samples = len(all_token_counts)
  latency_slo = float(args.latency_slo_tpot_ms)

  for in_bucket, out_bucket in sorted_keys:
    req_id = (
        f"req_in_{in_bucket.replace('-', '_')}_out_{out_bucket.replace('-', '_')}_tpot_{int(latency_slo)}ms"
    )
    request_types.append({
        "id": req_id,
        "latency_slo_tpot_ms": latency_slo,
        "input_size_bucket": in_bucket,
        "output_size_bucket": out_bucket,
    })
    count = bucket_counts[(in_bucket, out_bucket)]
    rate = (count / total_samples) * args.total_request_rate
    rate_distribution.append({"id": req_id, "rate": rate})

  return {
      "request_types": request_types,
      "rate_distribution": rate_distribution,
  }


def generate_cumulative_config(dataset_requests, hf_token):
  """Generates a cumulative request type and rate distribution from multiple datasets."""
  cumulative_request_types = []
  cumulative_rate_distribution = []

  for dataset_args in dataset_requests:
    dataset_args["hf_token"] = hf_token
    config_data = generate_config(argparse.Namespace(**dataset_args))    
    if not config_data:
      continue

    # Append request types, ensuring no duplicates
    for req_type in config_data["request_types"]:
      if req_type not in cumulative_request_types:
        cumulative_request_types.append(req_type)

    # Extend rate distribution
    cumulative_rate_distribution.extend(config_data["rate_distribution"])

  # Aggregate rates for the same request ID
  aggregated_rates = {}
  for item in cumulative_rate_distribution:
    req_id = item["id"]
    rate = item["rate"]
    if req_id in aggregated_rates:
      aggregated_rates[req_id] += rate
    else:
      aggregated_rates[req_id] = rate

  # Convert aggregated rates back to list
  final_rate_distribution = [
      {"id": req_id, "rate": rate} for req_id, rate in aggregated_rates.items()
  ]

  return {
      "request_types": cumulative_request_types,
      "rate_distribution": final_rate_distribution,
  }


def main():
  parser = argparse.ArgumentParser(
      description=(
          "Analyze a Hugging Face dataset to generate a request type and rate"
          " distribution file."
      ),
  )
  parser.add_argument(
      "--dataset_requests",
      required=True,
      type=str,
      help=(
          "JSON string of dataset requests. Each request should be a dictionary"
          " with dataset parameters."
      ),
  )
  parser.add_argument("--hf_token", type=str, default=None)
  parser.add_argument("--output_file", type=str, default="request_config.json")

  args = parser.parse_args()

  try:
    dataset_requests = json.loads(args.dataset_requests)
    if not isinstance(dataset_requests, list):
      raise ValueError("dataset_requests must be a JSON list.")
    for dataset_args in dataset_requests:
      if not isinstance(dataset_args, dict):
        raise ValueError("Each dataset request must be a JSON dictionary.")
      if "dataset_name" not in dataset_args:
        raise ValueError("dataset_name is required for each dataset request.")
      if "input_column" not in dataset_args:
        raise ValueError("input_column is required for each dataset request.")
      if "output_column" not in dataset_args:
        raise ValueError("output_column is required for each dataset request.")
      if "tokenizer_name" not in dataset_args:
        dataset_args["tokenizer_name"] = "meta-llama/Llama-3.1-8B-Instruct"
      if "latency_slo_tpot_ms" not in dataset_args:
        dataset_args["latency_slo_tpot_ms"] = 50.0
      if "total_request_rate" not in dataset_args:
        dataset_args["total_request_rate"] = 100.0
      if "dataset_config" not in dataset_args:
        dataset_args["dataset_config"] = None
      if "input_text_prefix" not in dataset_args:
        dataset_args["input_text_prefix"] = None
      if "max_samples" not in dataset_args:
        dataset_args["max_samples"] = None
  except json.JSONDecodeError:
    print("Error: Invalid JSON format for dataset_requests.")
    exit(1)
  except ValueError as e:
    print(f"Error: {e}")
    exit(1)

  config_data = generate_cumulative_config(dataset_requests, args.hf_token)

  if config_data:
    print(f"\nAnalysis complete. Saving configuration to {args.output_file}...")
    with open(args.output_file, "w") as f:
      json.dump(config_data, f, indent=2)
    print("File saved successfully.")
    print(f"Request types: {len(config_data['request_types'])}")
    print(f"Rate distribution: {len(config_data['rate_distribution'])}")
  else:
    print("No configuration was generated.")

if __name__ == "__main__":
  main()
