#!/bin/bash

# Load Test Runner Script
#
# Usage:
#   ./run-load-tests.sh baseline    # 5 minute baseline test
#   ./run-load-tests.sh spike       # Spike test
#   ./run-load-tests.sh soak        # 1 hour soak test
#   ./run-load-tests.sh stress      # Stress test
#   ./run-load-tests.sh all         # Run all tests sequentially

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
BASE_URL="${BASE_URL:-http://localhost:8080}"
RESULTS_DIR="${RESULTS_DIR:-./results}"

# Create results directory
mkdir -p "$RESULTS_DIR"

# Helper functions
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Check if server is running
check_server() {
    log_info "Checking if server is running at $BASE_URL..."

    if ! curl -s -f "$BASE_URL/health" > /dev/null 2>&1; then
        log_error "Server is not running at $BASE_URL"
        log_info "Start the server with: make run"
        exit 1
    fi

    log_success "Server is running"
}

# Check if k6 is installed
check_k6() {
    if ! command -v k6 &> /dev/null; then
        log_error "k6 is not installed"
        echo ""
        echo "Install k6:"
        echo "  macOS:  brew install k6"
        echo "  Linux:  sudo apt-get install k6"
        echo "  Other:  https://k6.io/docs/getting-started/installation/"
        echo ""
        echo "Alternatively, use Go-based tests:"
        echo "  go test -v ./tests/load -tags=loadtest"
        exit 1
    fi
}

# Run baseline test
run_baseline() {
    log_info "Running baseline load test (5 minutes)"
    log_info "Target: 1k-10k QPS with realistic traffic"

    k6 run \
        --out json="$RESULTS_DIR/baseline-$(date +%Y%m%d-%H%M%S).json" \
        tests/load/baseline.js

    log_success "Baseline test completed"
}

# Run spike test
run_spike() {
    log_info "Running spike test (2 minutes)"
    log_info "Pattern: Normal → 20x SPIKE → Recovery"

    k6 run \
        --out json="$RESULTS_DIR/spike-$(date +%Y%m%d-%H%M%S).json" \
        tests/load/spike.js

    log_success "Spike test completed"
}

# Run soak test
run_soak() {
    local duration="${1:-1h}"
    log_info "Running soak test ($duration)"
    log_warning "This will take a while. Press Ctrl+C to stop."

    k6 run \
        --duration "$duration" \
        --out json="$RESULTS_DIR/soak-$(date +%Y%m%d-%H%M%S).json" \
        tests/load/soak.js

    log_success "Soak test completed"
}

# Run stress test
run_stress() {
    log_info "Running stress test (10 minutes)"
    log_info "Finding the breaking point..."
    log_warning "Expect failures - that's the point!"

    k6 run \
        --out json="$RESULTS_DIR/stress-$(date +%Y%m%d-%H%M%S).json" \
        tests/load/stress.js

    log_success "Stress test completed"
}

# Run all tests
run_all() {
    log_info "Running all load tests sequentially"
    log_warning "This will take at least 1.5 hours"

    run_baseline
    sleep 30  # Cool down

    run_spike
    sleep 30  # Cool down

    run_soak "1h"
    sleep 30  # Cool down

    run_stress

    log_success "All tests completed!"
    log_info "Results saved in: $RESULTS_DIR"
}

# Display usage
usage() {
    echo "Load Test Runner"
    echo ""
    echo "Usage: $0 <test-type> [options]"
    echo ""
    echo "Test Types:"
    echo "  baseline    - 5 minute baseline test (1k-10k QPS)"
    echo "  spike       - 2 minute spike test (20x traffic burst)"
    echo "  soak        - Long-running stability test (default: 1 hour)"
    echo "  stress      - 10 minute stress test (find breaking point)"
    echo "  all         - Run all tests sequentially"
    echo ""
    echo "Examples:"
    echo "  $0 baseline"
    echo "  $0 soak 2h"
    echo "  BASE_URL=http://staging.example.com $0 baseline"
    echo ""
    echo "Environment Variables:"
    echo "  BASE_URL       - Target server URL (default: http://localhost:8080)"
    echo "  RESULTS_DIR    - Results directory (default: ./results)"
    exit 1
}

# Main
main() {
    if [ $# -eq 0 ]; then
        usage
    fi

    TEST_TYPE="$1"
    shift

    check_server
    check_k6

    case "$TEST_TYPE" in
        baseline)
            run_baseline
            ;;
        spike)
            run_spike
            ;;
        soak)
            run_soak "$@"
            ;;
        stress)
            run_stress
            ;;
        all)
            run_all
            ;;
        *)
            log_error "Unknown test type: $TEST_TYPE"
            usage
            ;;
    esac

    echo ""
    log_info "Next steps:"
    echo "  1. Review results in: $RESULTS_DIR"
    echo "  2. Check metrics: curl $BASE_URL/metrics"
    echo "  3. Check circuit breakers: curl $BASE_URL/admin/circuit-breakers"
    echo "  4. Compare against baselines in PERFORMANCE-BENCHMARKS.md"
}

main "$@"
