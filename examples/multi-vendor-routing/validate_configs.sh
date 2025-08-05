#!/bin/bash
# Configuration Validation Script for Multi-Vendor Routing
# Validates all Kubernetes configurations and provides comprehensive testing

set -e

echo "🔧 Multi-Vendor Routing Configuration Validation"
echo "================================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Validation counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Function to print test results
print_result() {
    local test_name="$1"
    local result="$2"
    local details="$3"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    if [ "$result" = "PASS" ]; then
        echo -e "${GREEN}✅ PASS${NC}: $test_name"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    elif [ "$result" = "FAIL" ]; then
        echo -e "${RED}❌ FAIL${NC}: $test_name"
        if [ -n "$details" ]; then
            echo -e "   ${RED}Error: $details${NC}"
        fi
        FAILED_TESTS=$((FAILED_TESTS + 1))
    elif [ "$result" = "WARN" ]; then
        echo -e "${YELLOW}⚠️  WARN${NC}: $test_name"
        if [ -n "$details" ]; then
            echo -e "   ${YELLOW}Warning: $details${NC}"
        fi
    else
        echo -e "${BLUE}ℹ️  INFO${NC}: $test_name"
        if [ -n "$details" ]; then
            echo -e "   ${BLUE}Info: $details${NC}"
        fi
    fi
}

# Check prerequisites
echo -e "\n${BLUE}🔍 Checking Prerequisites${NC}"
echo "-------------------------"

# Check kubectl
if command -v kubectl &> /dev/null; then
    KUBECTL_VERSION=$(kubectl version --client --short 2>/dev/null | cut -d' ' -f3)
    print_result "kubectl available" "PASS" "Version: $KUBECTL_VERSION"
else
    print_result "kubectl available" "FAIL" "kubectl not found in PATH"
    exit 1
fi

# Check Kubernetes cluster connectivity
if kubectl cluster-info &> /dev/null; then
    CLUSTER_VERSION=$(kubectl version --short 2>/dev/null | grep "Server Version" | cut -d' ' -f3)
    print_result "Kubernetes cluster connectivity" "PASS" "Server: $CLUSTER_VERSION"
else
    print_result "Kubernetes cluster connectivity" "FAIL" "Cannot connect to cluster"
    exit 1
fi

# Validate YAML syntax
echo -e "\n${BLUE}📝 YAML Syntax Validation${NC}"
echo "------------------------"

yaml_files=(
    "examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml"
    "examples/multi-vendor-routing/scenarios/cost-optimization.yaml"
    "examples/multi-vendor-routing/scenarios/high-performance.yaml"
    "examples/multi-vendor-routing/templates/monitoring/prometheus-rules.yaml"
)

for yaml_file in "${yaml_files[@]}"; do
    if [ -f "$yaml_file" ]; then
        # Check YAML syntax using kubectl
        if kubectl apply --dry-run=client -f "$yaml_file" &> /dev/null; then
            print_result "YAML syntax: $(basename $yaml_file)" "PASS"
        else
            error_msg=$(kubectl apply --dry-run=client -f "$yaml_file" 2>&1 | head -1)
            print_result "YAML syntax: $(basename $yaml_file)" "FAIL" "$error_msg"
        fi
    else
        print_result "File exists: $(basename $yaml_file)" "FAIL" "File not found"
    fi
done

# Validate Kubernetes resource definitions
echo -e "\n${BLUE}🎯 Kubernetes Resource Validation${NC}"
echo "--------------------------------"

# Check main deployment
if kubectl apply --dry-run=client -f "examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml" &> /dev/null; then
    resource_count=$(kubectl apply --dry-run=client -f "examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml" 2>/dev/null | grep -c "created (dry run)")
    print_result "Main deployment resources" "PASS" "$resource_count resources validated"
else
    print_result "Main deployment resources" "FAIL" "Resource validation failed"
fi

# Validate CRD dependencies
required_crds=(
    "inferencepools.llm-d.ai"
    "httproutes.gateway.networking.k8s.io"
    "gateways.gateway.networking.k8s.io"
)

for crd in "${required_crds[@]}"; do
    if kubectl get crd "$crd" &> /dev/null; then
        print_result "CRD available: $crd" "PASS"
    else
        print_result "CRD available: $crd" "WARN" "CRD not installed (expected for validation)"
    fi
done

# Validate resource quotas and limits
echo -e "\n${BLUE}💾 Resource Configuration Validation${NC}"
echo "-----------------------------------"

# Check NVIDIA resource requests
nvidia_gpu_request=$(grep -A 10 "nvidia.com/gpu" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml | grep -c "nvidia.com/gpu: 1" || echo "0")
if [ "$nvidia_gpu_request" -gt 0 ]; then
    print_result "NVIDIA GPU resource requests" "PASS" "$nvidia_gpu_request GPU requests configured"
else
    print_result "NVIDIA GPU resource requests" "FAIL" "No NVIDIA GPU requests found"
fi

# Check AMD resource requests  
amd_gpu_request=$(grep -A 10 "amd.com/gpu" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml | grep -c "amd.com/gpu: 1" || echo "0")
if [ "$amd_gpu_request" -gt 0 ]; then
    print_result "AMD GPU resource requests" "PASS" "$amd_gpu_request GPU requests configured"
else
    print_result "AMD GPU resource requests" "FAIL" "No AMD GPU requests found"
fi

# Validate memory and CPU requests
memory_requests=$(grep -c "memory:" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")
cpu_requests=$(grep -c "cpu:" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")

if [ "$memory_requests" -gt 4 ]; then
    print_result "Memory resource configuration" "PASS" "$memory_requests memory configurations"
else
    print_result "Memory resource configuration" "WARN" "Limited memory configurations found"
fi

if [ "$cpu_requests" -gt 4 ]; then
    print_result "CPU resource configuration" "PASS" "$cpu_requests CPU configurations"
else
    print_result "CPU resource configuration" "WARN" "Limited CPU configurations found"
fi

# Validate routing configuration
echo -e "\n${BLUE}🔀 Routing Configuration Validation${NC}"
echo "----------------------------------"

# Check HTTPRoute rules
route_rules=$(grep -c "matches:" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")
if [ "$route_rules" -gt 3 ]; then
    print_result "HTTPRoute routing rules" "PASS" "$route_rules routing rules configured"
else
    print_result "HTTPRoute routing rules" "WARN" "Limited routing rules found"
fi

# Check backend references
nvidia_backends=$(grep -c "nvidia-h100-pool" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")
amd_backends=$(grep -c "amd-mi300x-pool" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")

if [ "$nvidia_backends" -gt 3 ]; then
    print_result "NVIDIA backend references" "PASS" "$nvidia_backends references"
else
    print_result "NVIDIA backend references" "WARN" "Limited NVIDIA backend references"
fi

if [ "$amd_backends" -gt 3 ]; then
    print_result "AMD backend references" "PASS" "$amd_backends references"
else
    print_result "AMD backend references" "WARN" "Limited AMD backend references"
fi

# Validate monitoring configuration
echo -e "\n${BLUE}📊 Monitoring Configuration Validation${NC}"
echo "-------------------------------------"

# Check Prometheus rules
if [ -f "examples/multi-vendor-routing/templates/monitoring/prometheus-rules.yaml" ]; then
    alert_rules=$(grep -c "alert:" examples/multi-vendor-routing/templates/monitoring/prometheus-rules.yaml || echo "0")
    recording_rules=$(grep -c "record:" examples/multi-vendor-routing/templates/monitoring/prometheus-rules.yaml || echo "0")
    
    if [ "$alert_rules" -gt 10 ]; then
        print_result "Prometheus alert rules" "PASS" "$alert_rules alert rules configured"
    else
        print_result "Prometheus alert rules" "WARN" "Limited alert rules ($alert_rules found)"
    fi
    
    if [ "$recording_rules" -gt 3 ]; then
        print_result "Prometheus recording rules" "PASS" "$recording_rules recording rules configured"
    else
        print_result "Prometheus recording rules" "WARN" "Limited recording rules ($recording_rules found)"
    fi
else
    print_result "Prometheus rules file" "FAIL" "Monitoring rules file not found"
fi

# Check Grafana dashboard
if [ -f "examples/multi-vendor-routing/templates/monitoring/grafana-dashboard.json" ]; then
    dashboard_panels=$(grep -c '"id":' examples/multi-vendor-routing/templates/monitoring/grafana-dashboard.json || echo "0")
    if [ "$dashboard_panels" -gt 8 ]; then
        print_result "Grafana dashboard panels" "PASS" "$dashboard_panels panels configured"
    else
        print_result "Grafana dashboard panels" "WARN" "Limited dashboard panels ($dashboard_panels found)"
    fi
else
    print_result "Grafana dashboard file" "FAIL" "Dashboard file not found"
fi

# Validate security configuration
echo -e "\n${BLUE}🔒 Security Configuration Validation${NC}"
echo "-----------------------------------"

# Check security contexts
security_contexts=$(grep -c "securityContext:" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")
if [ "$security_contexts" -gt 2 ]; then
    print_result "Security contexts" "PASS" "$security_contexts security contexts configured"
else
    print_result "Security contexts" "WARN" "Limited security contexts found"
fi

# Check network policies
network_policies=$(grep -c "NetworkPolicy" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")
if [ "$network_policies" -gt 0 ]; then
    print_result "Network policies" "PASS" "$network_policies network policies configured"
else
    print_result "Network policies" "WARN" "No network policies found"
fi

# Check RBAC (if present)
rbac_configs=$(grep -c -E "(Role|ClusterRole|RoleBinding|ClusterRoleBinding)" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")
if [ "$rbac_configs" -gt 0 ]; then
    print_result "RBAC configurations" "PASS" "$rbac_configs RBAC resources configured"
else
    print_result "RBAC configurations" "INFO" "No RBAC resources (may be handled separately)"
fi

# Validate scenario configurations
echo -e "\n${BLUE}🎭 Scenario Configuration Validation${NC}"
echo "-----------------------------------"

scenarios=("cost-optimization" "high-performance")
for scenario in "${scenarios[@]}"; do
    scenario_file="examples/multi-vendor-routing/scenarios/${scenario}.yaml"
    if [ -f "$scenario_file" ]; then
        if kubectl apply --dry-run=client -f "$scenario_file" &> /dev/null; then
            rules_count=$(grep -c "matches:" "$scenario_file" || echo "0")
            print_result "Scenario: $scenario" "PASS" "$rules_count routing rules"
        else
            print_result "Scenario: $scenario" "FAIL" "Configuration validation failed"
        fi
    else
        print_result "Scenario: $scenario" "FAIL" "Scenario file not found"
    fi
done

# Documentation validation
echo -e "\n${BLUE}📚 Documentation Validation${NC}"
echo "---------------------------"

docs=(
    "examples/multi-vendor-routing/README.md"
    "examples/multi-vendor-routing/quick-start/setup-guide.md"
)

for doc in "${docs[@]}"; do
    if [ -f "$doc" ]; then
        word_count=$(wc -w < "$doc")
        if [ "$word_count" -gt 1000 ]; then
            print_result "Documentation: $(basename $doc)" "PASS" "$word_count words"
        else
            print_result "Documentation: $(basename $doc)" "WARN" "Brief documentation ($word_count words)"
        fi
    else
        print_result "Documentation: $(basename $doc)" "FAIL" "Documentation file not found"
    fi
done

# Performance and best practices validation
echo -e "\n${BLUE}⚡ Performance & Best Practices${NC}"
echo "------------------------------"

# Check for resource limits
limits_count=$(grep -c "limits:" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")
if [ "$limits_count" -gt 2 ]; then
    print_result "Resource limits configured" "PASS" "$limits_count limit configurations"
else
    print_result "Resource limits configured" "WARN" "Limited resource limits found"
fi

# Check for health checks
health_checks=$(grep -c -E "(livenessProbe|readinessProbe|startupProbe)" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")
if [ "$health_checks" -gt 4 ]; then
    print_result "Health checks configured" "PASS" "$health_checks health check configurations"
else
    print_result "Health checks configured" "WARN" "Limited health checks found"
fi

# Check for autoscaling
hpa_configs=$(grep -c "HorizontalPodAutoscaler" examples/multi-vendor-routing/quick-start/nvidia-amd-deployment.yaml || echo "0")
if [ "$hpa_configs" -gt 1 ]; then
    print_result "Autoscaling configured" "PASS" "$hpa_configs HPA configurations"
else
    print_result "Autoscaling configured" "WARN" "Limited autoscaling configurations"
fi

# Final summary
echo -e "\n${BLUE}📋 Validation Summary${NC}"
echo "==================="

success_rate=$((PASSED_TESTS * 100 / TOTAL_TESTS))

echo -e "Total Tests: ${BLUE}$TOTAL_TESTS${NC}"
echo -known "Passed: ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed: ${RED}$FAILED_TESTS${NC}"
echo -e "Success Rate: ${BLUE}$success_rate%${NC}"

if [ "$FAILED_TESTS" -eq 0 ]; then
    echo -e "\n${GREEN}🎉 All critical validations passed!${NC}"
    echo -e "${GREEN}✅ Configuration is ready for deployment${NC}"
    exit 0
elif [ "$success_rate" -gt 80 ]; then
    echo -e "\n${YELLOW}⚠️  Most validations passed with some warnings${NC}"
    echo -e "${YELLOW}📋 Review warnings before production deployment${NC}"
    exit 0
else
    echo -e "\n${RED}❌ Multiple validation failures detected${NC}"
    echo -e "${RED}🔧 Please fix the issues before proceeding${NC}"
    exit 1
fi