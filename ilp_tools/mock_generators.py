# mock_generators.py
"""Generates mock offline profiling data and request rate distributions.

Models throughput as Requests Per Second (RPS), where request size and SLO
significantly impact processing time and achievable rate.
"""

import random
from typing import Any, Dict, List, Optional, Tuple
import numpy as np

# Import the structures defined in data_structures.py
from data_structures import WorkerUnit, RequestType


# Type Aliases
ProfileData = Dict[
    Tuple[str, str], float
]  # (worker_id, req_type_id) -> max_throughput (RPS)
DistributionData = Dict[str, float]  # req_type_id -> requests_per_second (RPS)

# ==============================================================================
# Centralized Hyperparameters for Mock Data Generation
# ==============================================================================

MOCK_GENERATOR_CONFIG = {
    # --- Profile Generation (Throughput = Requests Per Second) ---
    'base_throughput_scalar': 1.0,  # Multiplier for accel_speed_factors
    'min_final_throughput': 0.01,  # Minimum RPS produced by profiling
    'random_variation_range': (
        1,
        1,
    ),  # Multiplicative random noise range (e.g., (0.9, 1.1))
    # Accelerator Speed Factors: Baseline RPS for 'M' Input / 'M' Output request.
    'accel_speed_factors': {
        'H100': 24.0,
        'TPU-V5e': 22.0,
        'A100': 18.0,
        'A10G': 7.0,
        'L4': 2.0,
        'DEFAULT': 1.0,
    },
    # Output Size RPS Multiplier: Relative RPS factor based on output size ('M'=1.0).
    'accel_output_size_efficiency': {
        'H100': {'XS': 6.0, 'S': 2.5, 'M': 1.0, 'L': 0.40, 'XL': 0.20},
        'TPU-V5e': {'XS': 5.9, 'S': 2.45, 'M': 1.0, 'L': 0.40, 'XL': 0.20},
        'A100': {'XS': 5.8, 'S': 2.4, 'M': 1.0, 'L': 0.41, 'XL': 0.21},
        'A10G': {'XS': 5.5, 'S': 2.2, 'M': 1.0, 'L': 0.42, 'XL': 0.22},
        'L4': {'XS': 5.0, 'S': 2.0, 'M': 1.0, 'L': 0.45, 'XL': 0.25},
        'DEFAULT': {'XS': 4.0, 'S': 1.8, 'M': 1.0, 'L': 0.50, 'XL': 0.30},
    },
    # Input Size RPS Multiplier: Relative RPS factor based on input size ('M'=1.0).
    'accel_input_size_efficiency': {
        'H100': {'XS': 1.15, 'S': 1.05, 'M': 1.0, 'L': 0.95, 'XL': 0.90},
        'TPU-V5e': {'XS': 1.17, 'S': 1.07, 'M': 1.0, 'L': 0.93, 'XL': 0.89},
        'A100': {'XS': 1.16, 'S': 1.06, 'M': 1.0, 'L': 0.94, 'XL': 0.89},
        'A10G': {'XS': 1.18, 'S': 1.08, 'M': 1.0, 'L': 0.92, 'XL': 0.88},
        'L4': {'XS': 1.20, 'S': 1.10, 'M': 1.0, 'L': 0.90, 'XL': 0.85},
        'DEFAULT': {'XS': 1.25, 'S': 1.12, 'M': 1.0, 'L': 0.88, 'XL': 0.80},
    },
    # SLO Penalty Params: Applies penalty factor = SLO / (SLO + HalfCut) to RPS.
    # Lower half-cut means tighter SLOs impact RPS more significantly.
    'accel_slo_half_cut_ms': {
        'H100': 200.0,
        'A100': 350.0,
        'TPU-V5e': 350.0,
        'A10G': 500.0,
        'L4': 600.0,
        'DEFAULT': 450.0,
    },
    'min_slo_penalty_factor': 0.05,  # Minimum penalty factor applied (floor)
    'model_server_factors': {
        'DEFAULT': 1.0
    },  # Factor for model server overhead
    # --- Distribution Generation ---
    'max_active_request_types': (
        60
    ),  # Target number of types with non-zero rates
    'distribution_bias_factor': 3.0,  # Multiplier for weights of biased types
    'distribution_min_rate': (
        0.01
    ),  # Minimum RPS per request type in distribution
}

# ==============================================================================
# Helper Functions
# ==============================================================================


def _get_speed_factor(accel_type: str, config: Dict[str, Any]) -> float:
  """Gets the base speed factor (M/M RPS) for the accelerator."""
  return config['accel_speed_factors'].get(
      accel_type, config['accel_speed_factors']['DEFAULT']
  )


def _get_size_efficiency_factor(
    accel_type: str,
    input_size_category: str,
    output_size_category: str,
    config: Dict[str, Any],
) -> float:
  """Gets the combined RPS multiplier based on input and output size categories."""
  input_efficiencies = config['accel_input_size_efficiency'].get(
      accel_type, config['accel_input_size_efficiency']['DEFAULT']
  )
  input_efficiency = input_efficiencies.get(input_size_category.upper(), 1.0)

  output_efficiencies = config['accel_output_size_efficiency'].get(
      accel_type, config['accel_output_size_efficiency']['DEFAULT']
  )
  output_efficiency = output_efficiencies.get(output_size_category.upper(), 1.0)

  combined_efficiency = input_efficiency * output_efficiency
  return combined_efficiency

def _get_slo_penalty_factor(
    accel_type: str, slo_ms: int, config: Dict[str, Any]
) -> float:
  """Calculates the performance penalty factor based on SLO using a hyperbolic curve."""
  if slo_ms <= 0:
    # Treat invalid SLO as incurring maximum penalty (minimum factor)
    return config['min_slo_penalty_factor']

  slo_half_cut = config['accel_slo_half_cut_ms'].get(
      accel_type, config['accel_slo_half_cut_ms']['DEFAULT']
  )
  denominator = float(slo_ms + slo_half_cut)
  if denominator <= 1e-6:  # Avoid division by zero or near-zero
    factor = 0.0
  else:
    factor = (
        slo_ms / denominator
    )  # Factor approaches 1 for large SLO, 0 for small SLO

  return max(config['min_slo_penalty_factor'], factor)


def _get_server_factor(server_type: str, config: Dict[str, Any]) -> float:
  """Gets the performance factor for the model server type."""
  return config['model_server_factors'].get(
      server_type, config['model_server_factors']['DEFAULT']
  )


def _apply_random_variation(tput: float, config: Dict[str, Any]) -> float:
  """Applies a random multiplicative variation to the throughput (RPS)."""
  min_rand, max_rand = config['random_variation_range']
  if min_rand == max_rand:
    return (
        tput * min_rand
    )  # Handles case where range is (1, 1) or similar fixed factor
  return tput * random.uniform(min_rand, max_rand)


def _calculate_single_throughput(
    worker: WorkerUnit,
    req_type: RequestType,
    config: Dict[str, Any] = MOCK_GENERATOR_CONFIG,
) -> float:
  """Calculates mock max throughput (RPS) combining various performance factors."""
  base_tput = config['base_throughput_scalar']
  speed_f = _get_speed_factor(worker.accelerator_type, config)
  size_f = _get_size_efficiency_factor(
      worker.accelerator_type,
      req_type.input_size_bucket,
      req_type.output_size_bucket,
      config,
  )
  count_f = float(worker.accelerator_count)
  server_f = _get_server_factor(worker.model_server_type, config)
  slo_f = _get_slo_penalty_factor(
      worker.accelerator_type, req_type.slo_ms, config
  )

  # Throughput = Base * Speed * SizeFactor * AccelCount * ServerFactor * SloFactor
  adjusted_tput = base_tput * speed_f * size_f * count_f * server_f * slo_f
  tput_with_variation = _apply_random_variation(adjusted_tput, config)
  final_tput = max(config['min_final_throughput'], tput_with_variation)

  return final_tput


# ==============================================================================
# Main Generator Functions
# ==============================================================================


def generate_mock_profile(
    worker_units: List[WorkerUnit],
    request_types: List[RequestType],
    config: Dict[str, Any] = MOCK_GENERATOR_CONFIG,
) -> ProfileData:
  """Generates mock profiling data (max RPS) for all worker/request pairs."""
  profile: ProfileData = {}
  print(
      f'Generating Mock Profile Data (RPS) for {len(request_types)} potential'
      ' request types...'
  )
  for worker in worker_units:
    for req_type in request_types:
      final_tput = _calculate_single_throughput(worker, req_type, config)
      profile[(worker.id, req_type.id)] = final_tput
  print('...Mock Profile Generation Complete.')
  return profile


def generate_mock_distribution(
    request_types: List[RequestType],
    total_rate: float = 100.0,  # Target total incoming RPS for the distribution
    size_bias: Optional[
        str
    ] = None,  # Bias towards 'XS', 'S', 'M', 'L', or 'XL'
    slo_bias: Optional[
        str
    ] = None,  # Bias towards 'low', 'medium', or 'high' SLO
    config: Dict[str, Any] = MOCK_GENERATOR_CONFIG,
) -> DistributionData:
  """Generates a mock request rate distribution (RPS) across a sampled subset of types."""
  distribution: DistributionData = {}
  num_total_types = len(request_types)
  if num_total_types == 0:
    return distribution

  max_active = config.get('max_active_request_types', num_total_types)
  max_active = min(
      num_total_types, max_active if max_active is not None else num_total_types
  )

  print(
      f'Generating Mock Distribution Data (Total Rate: {total_rate} RPS, Target'
      f' Active Types: {max_active}, Size Bias: {size_bias}, SLO Bias:'
      f' {slo_bias})...'
  )

  # --- Select Subset of Active Request Types ---
  if max_active < num_total_types and max_active > 0:
    active_request_types = random.sample(request_types, k=max_active)
  elif max_active <= 0:
    active_request_types = []
  else:  # max_active >= num_total_types
    active_request_types = request_types

  if not active_request_types:
    print(
        '...Mock Distribution Generation Complete (No active types selected).'
    )
    return distribution
  print(f'  Selected {len(active_request_types)} types to assign rates to.')

  # --- Generate weights ONLY for the active subset ---
  bias_factor = config['distribution_bias_factor']
  weights = []
  for req_type in active_request_types:
    weight = random.uniform(0.1, 1.0)  # Base random weight

    # Apply size bias if specified
    if size_bias:
      target_size = size_bias.upper()
      if (
          req_type.input_size_bucket.upper() == target_size
          or req_type.output_size_bucket.upper() == target_size
      ):
        weight *= bias_factor

    # Apply SLO bias if specified
    if slo_bias:
      if req_type.slo_ms < 500:
        req_slo_cat = 'low'
      elif req_type.slo_ms >= 1500:
        req_slo_cat = 'high'
      else:
        req_slo_cat = 'medium'
      if req_slo_cat == slo_bias.lower():
        weight *= bias_factor

    weights.append(max(1e-9, weight))  # Ensure positive weight

  total_weight = sum(weights)

  # --- Normalize weights and distribute total_rate ONLY among active types ---
  actual_total_rate = 0.0
  min_rate = config['distribution_min_rate']

  if total_weight <= 1e-9:  # Handle case where all weights are effectively zero
    rate_per_type = (
        total_rate / len(active_request_types) if active_request_types else 0
    )
    for req_type in active_request_types:
      # Apply min_rate floor even during even distribution
      distribution[req_type.id] = max(min_rate, rate_per_type)
      actual_total_rate += distribution[req_type.id]
  else:
    for i, req_type in enumerate(active_request_types):
      rate = (weights[i] / total_weight) * total_rate
      distribution[req_type.id] = max(min_rate, rate)  # Apply min_rate floor
      actual_total_rate += distribution[req_type.id]

  # --- Re-normalize distribution if min_rate floor caused total rate mismatch ---
  if abs(actual_total_rate - total_rate) > 1e-2 and actual_total_rate > 1e-6:
    renorm_factor = total_rate / actual_total_rate
    print(
        f'  Re-normalizing distribution by factor {renorm_factor:.3f} to match'
        ' target rate.'
    )
    actual_total_rate = 0
    temp_dist = {}
    for req_id in distribution:
      renormalized_rate = distribution[req_id] * renorm_factor
      # Apply min_rate again after renormalization
      final_rate = max(min_rate, renormalized_rate)
      temp_dist[req_id] = final_rate
      actual_total_rate += final_rate
    distribution = temp_dist

  print(
      f'  (Actual total rate generated across {len(distribution)} active types:'
      f' {actual_total_rate:.2f} req/s)'
  )
  print('...Mock Distribution Generation Complete.')
  return distribution
