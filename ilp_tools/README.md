To run a quick check that solver dependencies, run the mock solver:

```shell
python mock_solver.py
```

For a full visual demonstration of the ILP solver, open the `visualize_phase1.ipynb` notebook and run the code blocks.

To test the provisoner with real benchmarking data and HF datasets Once you have all services set up and running (see READMEs in Performance Profiler and Request Distribution Service), you can test a provisioning with:

```shell
python run_live_ilp_demo.py
```