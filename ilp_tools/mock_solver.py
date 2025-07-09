# main.py
"""Main script to run the static Phase 1 simulation.

Defines mock inputs, calls generators, runs the ILP solver, and prints results.
"""

# Import project modules
from data_structures import RequestType, WorkerUnit
from ilp_solver import solve_provisioning_ilp
from mock_generators import generate_mock_distribution, generate_mock_profile


def run_phase1_static_demo():
  """Orchestrates the Phase 1 static demo."""
  print('===== Running Phase 1: Static Foundation Demo =====')

  # --- 1. Define Mock Infrastructure & Request Types ---
  print('\n1. Defining Mock Worker Units and Request Types...')
  # Example Worker Units (MSRS)
  worker_units = [
      WorkerUnit(
          id='L4x1_TGI',
          accelerator_type='L4',
          accelerator_count=1,
          model_server_type='TGI',
          cost=1.0,
      ),
      WorkerUnit(
          id='L4x2_TGI',
          accelerator_type='L4',
          accelerator_count=2,
          model_server_type='TGI',
          cost=1.9,
      ),  # Slightly cheaper than 2x L4x1
      WorkerUnit(
          id='A100x1_VLLM',
          accelerator_type='A100',
          accelerator_count=1,
          model_server_type='VLLM',
          cost=3.0,
      ),
      WorkerUnit(
          id='H100x1_VLLM',
          accelerator_type='H100',
          accelerator_count=1,
          model_server_type='VLLM',
          cost=8.0,
      ),
      # Add more diverse units if needed
  ]

  # Example Request Types
  request_types = [
      RequestType(id='size_S_slo_1000ms', input_size_bucket='S', output_size_bucket='S', slo_ms=1000),
      RequestType(id='size_S_slo_300ms', input_size_bucket='S', output_size_bucket='S', slo_ms=300),
      RequestType(id='size_M_slo_1000ms', input_size_bucket='M', output_size_bucket='M', slo_ms=1000),
      RequestType(id='size_M_slo_500ms', input_size_bucket='M', output_size_bucket='M', slo_ms=500),
      RequestType(id='size_L_slo_2000ms', input_size_bucket='L', output_size_bucket='L', slo_ms=2000),
  ]
  # Create a map for easy lookup in the solver
  request_type_map = {rt.id: rt for rt in request_types}

  print(
      f'Defined {len(worker_units)} worker unit types and'
      f' {len(request_types)} request types.'
  )

  # --- 2. Generate Mock Data ---
  print('\n2. Generating Mock Profile and Distribution...')
  profile_data = generate_mock_profile(worker_units, request_types)
  # Example: Generate a distribution with ~50 total req/s
  distribution_data = generate_mock_distribution(request_types, total_rate=50.0)

  # --- (Optional) Print Generated Data for Verification ---
  # print("\n--- Mock Profile Data ---")
  # for key, value in profile_data.items():
  #     print(f"  {key}: {value:.2f} req/s")
  # print("\n--- Mock Distribution Data ---")
  # for key, value in distribution_data.items():
  #      print(f"  {key}: {value:.2f} req/s")
  # print("----------------------------")

  # --- 3. Run the ILP Solver ---
  print('\n3. Calling the ILP Solver...')
  # Adjust slice_factor if needed
  optimal_counts, slice_assignments, slices = solve_provisioning_ilp(
      worker_units=worker_units,
      profile_data=profile_data,
      distribution_data=distribution_data,
      slice_factor=3,  # slice_factor defaults to 2, or set explicitly e.g., slice_factor=3
  )

  # --- 4. Print Results ---
  print('\n4. ILP Solver Results:')
  if optimal_counts is not None:
    print('  Optimal Provisioning Plan:')
    total_final_cost = 0
    cost_map = {w.id: w.cost for w in worker_units}
    has_provisioned_units = False
    for worker_id, count in optimal_counts.items():
      if count > 0:
        print(f'   - {worker_id}: {count} instances')
        total_final_cost += count * cost_map[worker_id]
        has_provisioned_units = True
    if not has_provisioned_units:
      print('   - No units provisioned (possibly zero demand or infeasible).')
    else:
      print(f'\n  Estimated Total Cost based on plan: {total_final_cost:.2f}')

  else:
    print('  Solver failed to find an optimal solution.')

  print('\n===== Phase 1 Demo Complete =====')


if __name__ == '__main__':
  # Ensure OR-Tools is installed: pip install ortools
  run_phase1_static_demo()
