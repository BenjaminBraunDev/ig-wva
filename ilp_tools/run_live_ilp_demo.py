"""Main script to fetch live data from Request Distribution and Performance Profiler
services, then run the ILP solver to determine optimal provisioning.
"""

import json
import subprocess
from typing import Any, Dict, List, Optional, Tuple

# Assuming these modules are in the same directory or PYTHONPATH is correctly set up.
from data_structures import WorkerUnit
from ilp_solver import solve_provisioning_ilp


# Configuration
RDS_ENDPOINT = "localhost:8081"
PPS_ENDPOINT = "localhost:8080"
RDS_SERVICE_METHOD = "request_distribution.RequestDistributionGenerator/GetCurrentDistribution"
PPS_SERVICE_METHOD = "profiler_service.PerformanceProfileGenerator/GenerateProfile"

# Hardcoded worker types (including cost for ILP, model_name/image for PPS)
# Note: 'model_server_type' is used by ILP's WorkerUnit,
#       'model_name' and 'model_server_image' are used by PPS.
WORKER_UNITS_DATA = [
    {
        "id": "h100_80gb_worker_config1",
        "model_name": "some_model_name_h100",
        "accelerator_type": "NVIDIA_H100_80GB",
        "accelerator_count": 1,
        "model_server_type": "VLLM_H100",  # Example model server type
        "model_server_image": "some_server_image_tag_h100",
        "cost": 11.06125002, # GCE a3-highgpu-1g hourly price
        "max_limit": None,  # Optional max limit for ILP
    },
    {
        "id": "l4_worker_config1",
        "model_name": "some_model_name_l4",
        "accelerator_type": "NVIDIA_L4",
        "accelerator_count": 1,
        "model_server_type": "TGI_L4",  # Example model server type
        "model_server_image": "some_server_image_tag_l4",
        "cost": 0.70683228, # GCE g2-standard-4 hourly price
        "max_limit": None,  # Optional max limit for ILP
    },
]


def run_grpcurl(
    endpoint: str, service_method: str, data: Optional[Dict[str, Any]] = None
) -> Optional[Dict[str, Any]]:
  """Helper function to run a grpcurl command and return the JSON response."""
  process = None  # Initialize process to None
  cmd = ["grpcurl", "-plaintext"]
  if data:
    cmd.extend(["-d", json.dumps(data)])
  cmd.extend([endpoint, service_method])

  print(f"\nExecuting grpcurl command:\n{' '.join(cmd)}")

  try:
    process = subprocess.run(cmd, capture_output=True, text=True, check=True)
    print(f"Response from {service_method}:\n{process.stdout}")
    return json.loads(process.stdout)
  except subprocess.CalledProcessError as e:
    print(f"Error calling {service_method}: {e}")
    print(f"Stderr: {e.stderr}")
    return None
  except json.JSONDecodeError as e:
    print(f"Error decoding JSON response from {service_method}: {e}")
    print(f"Stdout: {process.stdout if process is not None else 'N/A'}")
    return None


def prepare_worker_types_for_pps(
    worker_units_data: List[Dict[str, Any]],
) -> List[Dict[str, Any]]:
  """Removes 'cost' and 'max_limit', keeps fields needed by PPS."""
  pps_workers = []
  for wu_data in worker_units_data:
    pps_worker = {
        "id": wu_data["id"],
        "model_name": wu_data["model_name"],
        "accelerator_type": wu_data["accelerator_type"],
        "accelerator_count": wu_data["accelerator_count"],
        "model_server_type": wu_data[
            "model_server_type"
        ],  # PPS uses this field
        "model_server_image": wu_data["model_server_image"],
    }
    pps_workers.append(pps_worker)
  return pps_workers


def run_live_ilp_demo():
  """Orchestrates fetching data and running the ILP solver."""
  print("===== Running Live ILP Demo =====")

  # 1. Call Request Distribution Service (RDS)
  print("\n--- Step 1: Fetching data from Request Distribution Service ---")
  rds_response = run_grpcurl(RDS_ENDPOINT, RDS_SERVICE_METHOD)

  if not rds_response:
    print("Failed to get data from RDS. Exiting.")
    return

  request_types_from_rds = rds_response.get("requestTypes", [])
  # Ensure correct field names for PPS if they differ (e.g. latencySloTpotMs)
  # The example matches, so direct use is fine.
  # request_types_for_pps = [
  # {
  # "id": rt.get("id"),
  # "input_size_bucket": rt.get("inputSizeBucket"),
  # "output_size_bucket": rt.get("outputSizeBucket"),
  # "latency_slo_tpot_ms": rt.get("latencySloTpotMs"),
  #     } for rt in request_types_from_rds
  # ]
  # Use request_types_from_rds directly as its structure matches what grpcurl needs

  distribution_data_for_ilp: Dict[str, float] = {}
  for item in rds_response.get("rateDistribution", []):
    req_id = item.get("requestTypeId")
    rate = item.get("rate")
    if req_id is not None and rate is not None:
      distribution_data_for_ilp[req_id] = float(rate)
    else:
      print(f"Warning: Skipping rate entry with missing data: {item}")

  if not request_types_from_rds:
    print("No request types received from RDS. Exiting.")
    return
  if not distribution_data_for_ilp:
    print("No rate distribution data received from RDS. Exiting.")
    return

  # 2. Call Performance Profiler Service (PPS)
  print("\n--- Step 2: Fetching data from Performance Profiler Service ---")
  worker_types_for_pps = prepare_worker_types_for_pps(WORKER_UNITS_DATA)
  pps_request_data = {
      "workload_definition": {
          "worker_types": worker_types_for_pps,
          "request_types": (
              request_types_from_rds  # Use the structure from RDS directly
          ),
      }
  }
  pps_response = run_grpcurl(PPS_ENDPOINT, PPS_SERVICE_METHOD, pps_request_data)

  if not pps_response:
    print("Failed to get data from PPS. Exiting.")
    return

  profile_data_for_ilp: Dict[Tuple[str, str], float] = {}
  if (
      pps_response
      and "performanceProfile" in pps_response
      and "entries" in pps_response["performanceProfile"]
  ):
    for entry in pps_response["performanceProfile"]["entries"]:
      worker_id = entry.get("workerTypeId")
      req_id = entry.get("requestTypeId")
      # Default to 0.0 if maxThroughputRps is missing or status indicates an issue
      throughput = entry.get("maxThroughputRps", 0.0)
      status = entry.get("status", "STATUS_UNSPECIFIED")

      if worker_id and req_id:
        # Only consider valid throughput if status is OK or OK_USING_HIGHEST_RATE
        if status in ["OK", "OK_USING_HIGHEST_RATE"]:
          profile_data_for_ilp[(worker_id, req_id)] = float(throughput)
        else:
          profile_data_for_ilp[(worker_id, req_id)] = (
              0.0  # Treat other statuses as 0 throughput for ILP
          )
          print(
              f"Info: Throughput for ({worker_id}, {req_id}) set to 0 due to"
              f" status: {status}"
          )
      else:
        print(f"Warning: Skipping profile entry with missing data: {entry}")
  else:
    print("Performance profile data is missing or malformed in PPS response.")
    # Depending on requirements, might exit or proceed with empty profile_data

  # 3. Prepare data for and call ILP Solver
  print("\n--- Step 3: Preparing data and calling ILP Solver ---")
  worker_units_for_ilp: List[WorkerUnit] = []
  for wu_data in WORKER_UNITS_DATA:
    worker_units_for_ilp.append(
        WorkerUnit(
            id=wu_data["id"],
            accelerator_type=wu_data["accelerator_type"],
            accelerator_count=wu_data["accelerator_count"],
            model_server_type=wu_data[
                "model_server_type"
            ],  # This field is in data_structures.WorkerUnit
            cost=wu_data["cost"],
            max_limit=wu_data.get("max_limit"),  # Use .get() for optional field
        )
    )

  print("\nWorker Units for ILP:")
  for wu in worker_units_for_ilp:
    print(f"  {wu}")
  print("\nProfile Data for ILP (WorkerID, RequestID -> Throughput):")
  for k, v in profile_data_for_ilp.items():
    print(f"  {k}: {v}")
  print("\nDistribution Data for ILP (RequestID -> Rate):")
  for k, v in distribution_data_for_ilp.items():
    print(f"  {k}: {v}")

  optimal_counts, slice_assignments, slices = solve_provisioning_ilp(
      worker_units=worker_units_for_ilp,
      profile_data=profile_data_for_ilp,
      distribution_data=distribution_data_for_ilp,
      # slice_factor=2 # Default is 2, can be overridden
  )

  # 4. Print ILP Solver Results
  print("\n--- Step 4: ILP Solver Results ---")
  if optimal_counts is not None:
    print("  Optimal Provisioning Plan:")
    total_final_cost = 0.0
    cost_map = {w.id: w.cost for w in worker_units_for_ilp}
    has_provisioned_units = False
    for worker_id, count in optimal_counts.items():
      if count > 0:
        print(
            f"    - {worker_id}: {count} instances (Cost per unit:"
            f" {cost_map.get(worker_id, 0):.2f})"
        )
        total_final_cost += count * cost_map.get(worker_id, 0)
        has_provisioned_units = True
    if not has_provisioned_units:
      print(
          "    - No units provisioned (possibly zero demand, infeasible, or no"
          " valid throughputs)."
      )
    else:
      print(f"\n  Estimated Total Cost based on plan: {total_final_cost:.2f}")

    # if slice_assignments:
    # print("\n  Slice Assignments (Slice ID -> Worker Type ID):")
    # for slice_id, worker_type_id in sorted(slice_assignments.items()):
    # print(f"    - Slice {slice_id}: {worker_type_id}")
    # else:
    # print("  No slice assignments reported (or all slices unassigned).")
  else:
    print("  Solver failed to find an optimal solution.")

  # print("\n  Generated Slices:")
  # for s_idx, sl in enumerate(slices):
  # print(f"    - Slice {sl.id}: RequestTypeID={sl.request_type_id}, RatePortion={sl.rate_portion:.2f}")
  # if not slices:
  # print("    - No slices were generated (e.g., zero total demand).")

  print("\n===== Live ILP Demo Complete =====")


if __name__ == "__main__":
  # Ensure `grpcurl` is installed and in PATH.
  # Ensure the RDS and PPS services are running on localhost:8081 and localhost:8080 respectively.
  run_live_ilp_demo()
