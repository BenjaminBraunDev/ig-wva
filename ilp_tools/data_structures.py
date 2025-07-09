"""Defines the basic data structures for Worker Units (MSRS), Request Types, and Slices.

RequestType now uses separate input/output size buckets. WorkerUnit includes
optional max_limit. Slice does not store SLO.
"""

from dataclasses import dataclass
from typing import Optional

@dataclass
class WorkerUnit:
  """Represents a type of Model Server Replica Set (MSRS)."""
  id: str
  accelerator_type: str
  accelerator_count: int
  model_server_type: str
  cost: float
  max_limit: Optional[int] = None  # Optional max instance count

@dataclass
class RequestType:
  """Represents a category of inference requests."""
  id: str  # Unique identifier (e.g., 'inS_outL_slo500')
  input_size_bucket: str  # e.g., 'XS', 'S', 'M', 'L', 'XL'
  output_size_bucket: str  # e.g., 'XS', 'S', 'M', 'L', 'XL'
  slo_ms: int  # Target latency SLO in milliseconds

@dataclass
class Slice:
  """Represents a portion of the request rate for a specific RequestType."""
  id: int  # Unique index for the slice
  request_type_id: str  # Links back to RequestType (which has size/SLO info)
  rate_portion: float  # Requests per second for this slice
