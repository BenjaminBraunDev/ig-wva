To run a quick check on the solver dependencies, run the mock solver:

```shell
python mock_solver.py
```

For a full visual demonstration of the ILP solver, open the `visualize_phase1.ipynb` notebook and run the code blocks.

To test the provisoner with real benchmarking data and HF datasets, set up both the Performance Profiler and Request Distribution Service (see READMEs in Performance Profiler and Request Distribution Service). Use the `example_l4_h100_bm_data.csv` when setting up the performance profiler, unless you have your own benchmarking data.

Once you have both services running, you can update the Request Rate Distribution with the following command. This includes an example set of HF datasets and rates to generate a diverse RRD (this may take a minute or two to update):

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
      "latency_slo_tpot_ms": 30.0,
      "max_samples": 0
    },
    {
      "dataset_name": "FiscalNote/billsum",
      "tokenizer_name": "meta-llama/Llama-3.1-8B-Instruct",
      "input_column": "text",
      "output_column": "summary",
      "dataset_config": "default",
      "total_request_rate": 100.0,
      "latency_slo_tpot_ms": 200.0,
      "max_samples": 0
    },
    {
      "dataset_name": "FiscalNote/billsum",
      "tokenizer_name": "meta-llama/Llama-3.1-8B-Instruct",
      "input_column": "text",
      "output_column": "summary",
      "dataset_config": "default",
      "total_request_rate": 100.0,
      "latency_slo_tpot_ms": 100.0,
      "max_samples": 0
    }
  ]
}' \
localhost:8081 request_distribution.RequestDistributionGenerator/UpdateDatasetAndRates
```

You can test a provisioning with:

```shell
python run_live_ilp_demo.py
```

Feel free to modify the HF datasets and rates and see how WVA provisions around the different requests and SLOs.