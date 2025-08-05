# Multi-Vendor Setup Guide

This guide walks you through deploying the multi-vendor routing reference architecture step-by-step.

## 🎯 Prerequisites

### 1. Kubernetes Cluster Requirements

```bash
# Minimum cluster specifications
# - Kubernetes 1.26+
# - At least 2 GPU node types (NVIDIA + AMD)
# - 200GB+ total cluster storage
# - LoadBalancer or Ingress controller

# Verify cluster readiness
kubectl version --short
kubectl get nodes -o wide
```

### 2. GPU Node Preparation

**NVIDIA Node Setup:**
```bash
# Ensure NVIDIA GPU Operator is installed
kubectl get pods -n gpu-operator-resources

# Verify NVIDIA node labels
kubectl get nodes -l accelerator=nvidia-h100 -o wide
kubectl get nodes -l nvidia.com/gpu.present=true

# Expected output:
# NAME            STATUS   ROLES    AGE   VERSION   ACCELERATOR
# nvidia-node-1   Ready    <none>   1d    v1.28.3   nvidia-h100
```

**AMD Node Setup:**
```bash
# Ensure AMD GPU device plugin is installed
kubectl get daemonset -n kube-system amd-gpu-device-plugin

# Verify AMD node labels  
kubectl get nodes -l accelerator=amd-mi300x -o wide
kubectl get nodes -l amd.com/gpu.present=true

# Expected output:
# NAME         STATUS   ROLES    AGE   VERSION   ACCELERATOR
# amd-node-1   Ready    <none>   1d    v1.28.3   amd-mi300x
```

### 3. Install llm-d and Dependencies

```bash
# Install llm-d framework
kubectl apply -f https://github.com/llm-d/llm-d/releases/latest/download/install.yaml

# Install Inference Gateway
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/latest/download/install.yaml

# Verify installations
kubectl get crd | grep -E "(inference|gateway)"
kubectl get pods -n llm-d-system
kubectl get pods -n gateway-system
```

## 🚀 Step-by-Step Deployment

### Step 1: Node Labeling and Preparation

```bash
# Label NVIDIA nodes for high-performance tier
kubectl label nodes <nvidia-node-name> accelerator=nvidia-h100
kubectl label nodes <nvidia-node-name> tier=premium

# Label AMD nodes for cost-optimized tier  
kubectl label nodes <amd-node-name> accelerator=amd-mi300x
kubectl label nodes <amd-node-name> tier=standard

# Verify labels
kubectl get nodes --show-labels | grep accelerator
```

### Step 2: Deploy Multi-Vendor Configuration

```bash
# Clone the configuration
git clone https://github.com/llm-d-incubation/ig-wva.git
cd ig-wva/examples/multi-vendor-routing

# Apply the complete configuration
kubectl apply -f quick-start/nvidia-amd-deployment.yaml

# Monitor deployment progress
kubectl get pods -n llm-d-multi-vendor -w
```

**Expected deployment timeline:**

| Phase | Duration | Status Check |
|-------|----------|--------------|
| Namespace creation | 5s | `kubectl get ns llm-d-multi-vendor` |
| InferencePool creation | 30s | `kubectl get inferencepools -n llm-d-multi-vendor` |
| Container image pulls | 3-5 min | `kubectl describe pods -n llm-d-multi-vendor` |
| Model downloads | 5-10 min | `kubectl logs -f -n llm-d-multi-vendor <pod-name>` |
| Health checks passing | 2-3 min | `kubectl get pods -n llm-d-multi-vendor` |

### Step 3: Verify Deployment Status

```bash
# Check all components are running
kubectl get all -n llm-d-multi-vendor

# Expected output:
# NAME                                      READY   STATUS    RESTARTS   AGE
# pod/nvidia-h100-pool-xxx                  1/1     Running   0          5m
# pod/amd-mi300x-pool-yyy                   1/1     Running   0          7m
# pod/amd-mi300x-pool-zzz                   1/1     Running   0          7m

# NAME                          TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)
# service/nvidia-h100-pool      ClusterIP   10.96.xx.xx     <none>        8000/TCP,8001/TCP
# service/amd-mi300x-pool       ClusterIP   10.96.yy.yy     <none>        8000/TCP,8001/TCP

# Verify inference pools
kubectl get inferencepools -n llm-d-multi-vendor -o wide

# Check routing configuration
kubectl get httproute -n llm-d-multi-vendor
kubectl describe httproute multi-vendor-routing -n llm-d-multi-vendor
```

### Step 4: Test Connectivity and Health

```bash
# Test NVIDIA pool health
kubectl port-forward -n llm-d-multi-vendor svc/nvidia-h100-pool 8000:8000 &
curl http://localhost:8000/health
# Expected: {"status": "ready"}

# Test AMD pool health  
kubectl port-forward -n llm-d-multi-vendor svc/amd-mi300x-pool 8001:8000 &
curl http://localhost:8001/health  
# Expected: {"status": "ready"}

# Clean up port forwards
pkill -f "kubectl port-forward"
```

## 🔧 Testing Multi-Vendor Routing

### Test 1: High-Priority Request (Should Route to NVIDIA)

```bash
# Get gateway external IP
GATEWAY_IP=$(kubectl get svc -n llm-d-multi-vendor multi-vendor-gateway-svc -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Test high-priority routing
curl -X POST "http://${GATEWAY_IP}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "X-Priority: high" \
  -H "X-SLA-Target: p99-200ms" \
  -d '{
    "model": "llama-70b-chat",
    "messages": [
      {
        "role": "user", 
        "content": "Explain the benefits of multi-vendor GPU deployment for LLM inference."
      }
    ],
    "max_tokens": 150,
    "temperature": 0.7
  }'

# Check response headers for routing information
# Look for: X-Backend-Used: nvidia-h100-pool
```

### Test 2: Cost-Optimized Request (Should Route to AMD)

```bash
# Test cost-optimized routing
curl -X POST "http://${GATEWAY_IP}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "X-Cost-Tier: standard" \
  -H "X-SLA-Target: p95-500ms" \
  -d '{
    "model": "llama-7b-chat",
    "messages": [
      {
        "role": "user",
        "content": "What are the advantages of AMD GPUs for AI workloads?"
      }
    ],
    "max_tokens": 100,
    "temperature": 0.5
  }'

# Check response headers for routing information  
# Look for: X-Backend-Used: amd-mi300x-pool
```

### Test 3: Load Balancing and Failover

```bash
# Create load test script
cat << 'EOF' > load_test.sh
#!/bin/bash
GATEWAY_IP=$1
for i in {1..50}; do
  curl -s -X POST "http://${GATEWAY_IP}/v1/completions" \
    -H "Content-Type: application/json" \
    -d '{
      "model": "llama-7b-chat",
      "prompt": "Test request #'$i'",
      "max_tokens": 20
    }' | jq -r '.choices[0].text' &
  
  if (( i % 10 == 0 )); then
    wait  # Wait every 10 requests
    echo "Completed $i requests"
  fi
done
wait
EOF

# Run load test
chmod +x load_test.sh
./load_test.sh ${GATEWAY_IP}

# Monitor routing distribution
kubectl logs -f -n llm-d-multi-vendor deployment/inference-gateway | grep "route_decision"
```

## 📊 Monitoring and Observability

### Set Up Monitoring Dashboard

```bash
# Deploy Prometheus (if not already installed)
kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/bundle.yaml

# Port forward to Prometheus
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090 &

# Open Prometheus UI
echo "Prometheus available at: http://localhost:9090"
```

### Key Metrics to Monitor

```promql
# Routing decision distribution
sum(rate(ig_wva_routing_decisions_total[5m])) by (backend_vendor)

# Request latency by vendor
histogram_quantile(0.95, sum(rate(ig_wva_request_duration_seconds_bucket[5m])) by (le, backend_vendor))

# GPU utilization
avg(nvidia_smi_utilization_gpu_ratio) by (node)
avg(rocm_smi_utilization_gpu_ratio) by (node)

# Cost savings tracking
sum(rate(ig_wva_cost_savings_total[5m])) by (vendor_pair)

# Queue lengths by pool
avg(vllm_queue_length) by (inference_pool)
```

### Create Custom Dashboard

```bash
# Download Grafana dashboard template
curl -O https://raw.githubusercontent.com/llm-d-incubation/ig-wva/main/examples/multi-vendor-routing/monitoring/grafana-dashboard.json

# Import to Grafana
kubectl port-forward -n monitoring svc/grafana 3000:3000 &
# Visit http://localhost:3000, login (admin/admin), import dashboard
```

## 🛠️ Troubleshooting Common Issues

### Issue 1: Pods Stuck in Pending State

```bash
# Check node resources and scheduling
kubectl describe pod -n llm-d-multi-vendor <pod-name>

# Common causes and fixes:
# 1. Insufficient GPU resources
kubectl get nodes -o yaml | grep -A 5 -B 5 "nvidia.com/gpu\|amd.com/gpu"

# 2. Node selector not matching
kubectl get nodes --show-labels | grep accelerator

# 3. Tolerations missing
kubectl describe node <node-name> | grep -A 10 Taints
```

### Issue 2: Model Download Failures

```bash  
# Check model download logs
kubectl logs -n llm-d-multi-vendor <pod-name> --previous

# Common fixes:
# 1. Increase storage limits
kubectl patch inferencepools nvidia-h100-pool -n llm-d-multi-vendor --type='merge' -p='{"spec":{"template":{"spec":{"volumes":[{"name":"model-cache","emptyDir":{"sizeLimit":"150Gi"}}]}}}}'

# 2. Check network policies
kubectl get networkpolicy -n llm-d-multi-vendor
kubectl describe networkpolicy multi-vendor-network-policy -n llm-d-multi-vendor

# 3. Verify internet access
kubectl run debug --image=busybox -n llm-d-multi-vendor -it --rm -- wget -O- https://huggingface.co
```

### Issue 3: ROCm Initialization Failures (AMD)

```bash
# Check ROCm setup in AMD pods
kubectl exec -n llm-d-multi-vendor <amd-pod-name> -- rocm-smi

# Common fixes:
# 1. Verify ROCm device plugin
kubectl get daemonset -n kube-system | grep amd

# 2. Check GPU visibility
kubectl exec -n llm-d-multi-vendor <amd-pod-name> -- env | grep ROCR

# 3. Disable problematic features
kubectl set env deployment/amd-mi300x-pool -n llm-d-multi-vendor VLLM_USE_TRITON_FLASH_ATTN=0
```

### Issue 4: Routing Not Working as Expected

```bash
# Check gateway status
kubectl get gateway -n llm-d-multi-vendor
kubectl describe gateway multi-vendor-gateway -n llm-d-multi-vendor

# Verify routing rules
kubectl get httproute -n llm-d-multi-vendor -o yaml

# Check service endpoints
kubectl get endpoints -n llm-d-multi-vendor

# Test individual services
kubectl port-forward -n llm-d-multi-vendor svc/nvidia-h100-pool 8080:8000 &
curl -H "Host: test.local" http://localhost:8080/v1/models
```

## 🔄 Next Steps

### Performance Optimization

```bash
# 1. Adjust resource allocations based on monitoring
kubectl edit inferencepools nvidia-h100-pool -n llm-d-multi-vendor

# 2. Fine-tune autoscaling parameters
kubectl edit hpa nvidia-h100-hpa -n llm-d-multi-vendor

# 3. Optimize routing weights based on performance data
kubectl edit httproute multi-vendor-routing -n llm-d-multi-vendor
```

### Add More Scenarios

```bash
# Deploy additional routing scenarios
kubectl apply -f scenarios/cost-optimization.yaml
kubectl apply -f scenarios/high-performance.yaml
kubectl apply -f scenarios/balanced-routing.yaml
```

### Production Hardening

```bash
# Add security policies
kubectl apply -f templates/security/pod-security-policies.yaml
kubectl apply -f templates/security/rbac-configs.yaml

# Configure backup and disaster recovery
kubectl apply -f templates/backup/persistent-volumes.yaml
kubectl apply -f templates/backup/backup-policies.yaml
```

## 📚 Further Reading

- [Advanced Routing Patterns](../scenarios/README.md)
- [Monitoring and Alerting](../templates/monitoring/README.md)  
- [Security Best Practices](../templates/security/README.md)
- [Performance Tuning Guide](../benchmarks/README.md)

---

**Congratulations!** 🎉 You now have a fully functional multi-vendor LLM inference system with intelligent routing, cost optimization, and automatic failover capabilities.