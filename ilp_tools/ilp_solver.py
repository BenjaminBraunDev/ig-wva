"""
Formulates and solves the provisioning ILP using OR-Tools.
Includes optional maximum limits for worker types.
Returns optimal counts, slice assignments, and the list of slices.
"""

import math
from typing import Any, Dict, List, Optional, Tuple
from ortools.linear_solver import pywraplp
from data_structures import WorkerUnit, Slice


# Type Aliases
ProfileData = Dict[Tuple[str, str], float]
DistributionData = Dict[str, float]
OptimalCounts = Optional[Dict[str, int]]
SliceAssignments = Optional[Dict[int, str]]
SliceList = List[Any]
SolverResult = Tuple[OptimalCounts, SliceAssignments, SliceList]


def solve_provisioning_ilp(
    worker_units: List[WorkerUnit],
    profile_data: ProfileData,
    distribution_data: DistributionData,
    slice_factor: int = 2
) -> SolverResult:
    """
    Solves the ILP to find the minimum cost worker unit provisioning,
    respecting optional maximum limits per worker type.
    """
    print("\n--- Starting ILP Solver (with Max Limit Constraint) ---")

    # --- 1. Create Slices ---
    slices: SliceList = []
    slice_id_counter = 0
    print(f"Creating slices (slice_factor={slice_factor})...")
    for req_type_id, total_rate in distribution_data.items():
        if total_rate <= 0: continue
        if slice_factor <= 0: slice_factor = 1
        rate_portion = total_rate / slice_factor
        if rate_portion < 1e-6:
            if total_rate > 1e-9: rate_portion = total_rate; current_slice_factor = 1
            else: continue
        else: current_slice_factor = slice_factor
        for i in range(current_slice_factor):
            current_slice_id = slice_id_counter + i
            slices.append(Slice(id=current_slice_id, request_type_id=req_type_id, rate_portion=rate_portion))
        slice_id_counter += current_slice_factor
    num_slices = len(slices)
    if num_slices == 0:
        print("No requests/slices. Returning empty provisioning.")
        return ({w.id: 0 for w in worker_units}, {}, slices)
    print(f"...Created {num_slices} slices.")

    # --- 2. Calculate Load Matrix L_ij ---
    print("Calculating load matrix L_ij...")
    load_matrix: Dict[Tuple[int, str], float] = {}
    valid_profile_keys = set(profile_data.keys())
    for s in slices:
        for w in worker_units:
            profile_key = (w.id, s.request_type_id)
            load = float('inf')
            if profile_key in valid_profile_keys:
                max_tput = profile_data[profile_key]
                if max_tput > 0: load = s.rate_portion / max_tput
            load_matrix[(s.id, w.id)] = load
    print("...Load matrix calculated.")

    # --- 3. Setup OR-Tools Solver ---
    solver = pywraplp.Solver.CreateSolver('SCIP')
    if not solver:
        print("ERROR: No suitable MIP solver backend (SCIP) found.")
        return (None, None, slices)
    print(f"Using solver: {solver.SolverVersion()}")

    infinity = solver.infinity()

    # --- 4. Declare ILP Variables ---
    print("Declaring ILP variables...")
    # B_j: Number of workers of type j to allocate
    B = {w.id: solver.IntVar(0, infinity, f'B_{w.id}') for w in worker_units}
    # A_ij: 1 if slice i is assigned to worker type j, 0 otherwise
    A = {}
    assignable_slices_exist = False
    for s in slices:
        slice_can_be_assigned = False
        for w in worker_units:
            if load_matrix.get((s.id, w.id), float('inf')) != float('inf'):
                 A[(s.id, w.id)] = solver.BoolVar(f'A_{s.id}_{w.id}')
                 slice_can_be_assigned = True
                 assignable_slices_exist = True
        if not slice_can_be_assigned:
            print(f"Warning: Slice {s.id} ({s.request_type_id}) cannot be assigned to any worker type. Problem may be infeasible.")
    if not assignable_slices_exist and num_slices > 0:
         print("Error: No slices assignable. Problem likely infeasible.")
    # print("...Variables declared.")

    # --- 5. Define Constraints ---
    print("Defining constraints...")
    # Constraint 1: Slice assignment
    # print("  Constraint 1: Slice assignment...") # Less verbose
    for s in slices:
        assigned_workers = [A[(s.id, w.id)] for w in worker_units if (s.id, w.id) in A]
        if assigned_workers: solver.Add(sum(assigned_workers) == 1, f'Assign_{s.id}')

    # Constraint 2: Worker capacity
    # print("  Constraint 2: Worker capacity...") # Less verbose
    for w in worker_units:
        total_load_on_worker = [ A[(s.id, w.id)] * load_matrix[(s.id, w.id)] for s in slices if (s.id, w.id) in A ]
        if total_load_on_worker: solver.Add(sum(total_load_on_worker) <= B[w.id], f'Cap_{w.id}')
    print("  Constraint 3: Worker maximum limits (if specified)...")
    limit_constraints_added = 0
    for w in worker_units:
        # Check if max_limit attribute exists and is a valid non-negative integer
        if hasattr(w, 'max_limit') and w.max_limit is not None and w.max_limit >= 0:
            solver.Add(B[w.id] <= w.max_limit, f'Limit_{w.id}')
            limit_constraints_added += 1
            print(f"    - Added: {w.id} count <= {w.max_limit}")
    if limit_constraints_added == 0:
        print("    - No maximum limits specified for any worker type.")

    print("...Constraints defined.")

    # --- 6. Define Objective Function ---
    # print("Defining objective function (Minimize Cost)...") # Less verbose
    objective = solver.Objective()
    for w in worker_units: objective.SetCoefficient(B[w.id], w.cost)
    objective.SetMinimization()
    # print("...Objective defined.")

    # --- 7. Solve the ILP ---
    print("Solving the ILP...")
    status = solver.Solve()
    print("...Solver finished.")

    # --- 8. Process and Return Results ---
    if status == pywraplp.Solver.OPTIMAL:
        print("Optimal solution found!")
        optimal_counts = {}
        slice_assignments = {}
        total_cost = objective.Value()
        print(f"  Minimum total cost = {total_cost:.2f}")
        # Get counts B_j
        for w in worker_units:
            count = math.ceil(B[w.id].solution_value() - 1e-6)
            optimal_counts[w.id] = count
            # Print only allocated counts
            if count > 0: print(f"  Allocate {count} instances of {w.id}")

        # Get assignments A_ij
        assigned_slice_count = 0
        for s in slices:
            for w in worker_units:
                 if (s.id, w.id) in A and A[(s.id, w.id)].solution_value() > 0.5:
                     slice_assignments[s.id] = w.id
                     assigned_slice_count += 1
                     break
        # print(f"  ...extracted assignments for {assigned_slice_count}/{num_slices} slices.") # Less verbose
        print("--- ILP Solver Finished Successfully ---")
        return (optimal_counts, slice_assignments, slices)
    else:
        # Handle non-optimal cases (same as before)
        status_map = { pywraplp.Solver.FEASIBLE: "Feasible (suboptimal)", pywraplp.Solver.INFEASIBLE: "Infeasible",
                       pywraplp.Solver.UNBOUNDED: "Unbounded", pywraplp.Solver.ABNORMAL: "Abnormal",
                       pywraplp.Solver.NOT_SOLVED: "Not Solved", pywraplp.Solver.MODEL_INVALID: "Model Invalid"}
        print(f"ERROR: Optimal solution NOT found. Solver status code: {status} ({status_map.get(status, 'Unknown')})")
        print("--- ILP Solver Finished Unsuccessfully ---")
        return (None, None, slices)
