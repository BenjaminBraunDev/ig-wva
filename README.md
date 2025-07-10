# Workload Variant Autoscaler
WVA is a service to compute the cost-optimal provisioning of heterogeneous accelerators for inference workloads with varying request latency objectives

To test the pipeline E2E, have a look at the `ilp_tools/README.md` for E2E setup and running. The service finds the cost-optimal (currently using hardcoded GCE accelerator pricing) provisioning given your benchmarking data and choosen HF datasets to represent the request rate distribution.
