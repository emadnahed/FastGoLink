#!/usr/bin/env bash
#
# Load Testing Script for GoURL
# Tests latency and concurrency performance
#
# Usage:
#   ./scripts/loadtest.sh                    # Run against localhost:8080
#   ./scripts/loadtest.sh http://server:8080 # Run against custom server
#   ./scripts/loadtest.sh --quick            # Quick test (fewer requests)
#   ./scripts/loadtest.sh --full             # Full test (more requests)
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'
BOLD='\033[1m'

# Default configuration
BASE_URL="${1:-http://localhost:8080}"
MODE="standard"
CONCURRENCY=50
REQUESTS=1000
WARMUP_REQUESTS=50

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --quick)
            MODE="quick"
            CONCURRENCY=10
            REQUESTS=100
            shift
            ;;
        --full)
            MODE="full"
            CONCURRENCY=100
            REQUESTS=5000
            shift
            ;;
        --stress)
            MODE="stress"
            CONCURRENCY=200
            REQUESTS=10000
            shift
            ;;
        http://*|https://*)
            BASE_URL="$1"
            shift
            ;;
        --help|-h)
            echo "GoURL Load Testing Script"
            echo ""
            echo "Usage: $0 [URL] [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --quick     Quick test (10 concurrent, 100 requests)"
            echo "  --full      Full test (100 concurrent, 5000 requests)"
            echo "  --stress    Stress test (200 concurrent, 10000 requests)"
            echo "  --help      Show this help"
            echo ""
            echo "Examples:"
            echo "  $0                           # Test localhost:8080"
            echo "  $0 http://myserver:8080      # Test custom server"
            echo "  $0 --quick                   # Quick test"
            echo ""
            exit 0
            ;;
        *)
            shift
            ;;
    esac
done

# Remove trailing slash
BASE_URL="${BASE_URL%/}"

print_header() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}${CYAN}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
}

print_section() {
    echo ""
    echo -e "${YELLOW}───────────────────────────────────────────────────────────────${NC}"
    echo -e "${BOLD}  $1${NC}"
    echo -e "${YELLOW}───────────────────────────────────────────────────────────────${NC}"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_info() {
    echo -e "${CYAN}ℹ${NC} $1"
}

# Check if server is reachable
check_server() {
    print_section "Checking Server Connectivity"

    if curl -s --connect-timeout 5 "${BASE_URL}/health" > /dev/null 2>&1; then
        print_success "Server is reachable at ${BASE_URL}"
        return 0
    else
        print_error "Cannot connect to ${BASE_URL}"
        print_info "Make sure the server is running:"
        print_info "  make run"
        print_info "  # or"
        print_info "  docker-compose up -d"
        exit 1
    fi
}

# Test health endpoint latency
test_health_latency() {
    print_section "Health Endpoint Latency Test"

    local total_time=0
    local count=20
    local times=()

    print_info "Running $count requests to /health..."

    for i in $(seq 1 $count); do
        local time_ms=$(curl -s -o /dev/null -w "%{time_total}" "${BASE_URL}/health" | awk '{printf "%.3f", $1 * 1000}')
        times+=("$time_ms")
        total_time=$(echo "$total_time + $time_ms" | bc)
    done

    # Sort times
    IFS=$'\n' sorted=($(sort -n <<<"${times[*]}")); unset IFS

    local avg=$(echo "scale=3; $total_time / $count" | bc)
    local min=${sorted[0]}
    local max=${sorted[$((count-1))]}
    local p50=${sorted[$((count/2))]}
    local p95=${sorted[$((count*95/100))]}
    local p99=${sorted[$((count*99/100))]}

    echo ""
    echo -e "  ${BOLD}Min:${NC}     ${min}ms"
    echo -e "  ${BOLD}Avg:${NC}     ${avg}ms"
    echo -e "  ${BOLD}P50:${NC}     ${p50}ms"
    echo -e "  ${BOLD}P95:${NC}     ${p95}ms"
    echo -e "  ${BOLD}P99:${NC}     ${p99}ms"
    echo -e "  ${BOLD}Max:${NC}     ${max}ms"

    if (( $(echo "$p99 < 50" | bc -l) )); then
        print_success "Health endpoint P99 latency is under 50ms"
    else
        print_error "Health endpoint P99 latency is high: ${p99}ms"
    fi
}

# Create test URL and test redirect latency
test_redirect_latency() {
    print_section "Redirect Latency Test"

    # Create a test URL
    print_info "Creating test URL..."

    local response=$(curl -s -X POST "${BASE_URL}/api/v1/shorten" \
        -H "Content-Type: application/json" \
        -d '{"url":"https://example.com/loadtest"}')

    local short_code=$(echo "$response" | grep -o '"short_code":"[^"]*"' | cut -d'"' -f4)

    if [ -z "$short_code" ]; then
        print_error "Failed to create test URL"
        print_info "Response: $response"
        return 1
    fi

    print_success "Created short URL: ${short_code}"

    # Warmup
    print_info "Warming up (${WARMUP_REQUESTS} requests)..."
    for i in $(seq 1 $WARMUP_REQUESTS); do
        curl -s -o /dev/null -w "" "${BASE_URL}/${short_code}" -L --max-redirs 0 2>/dev/null || true
    done

    # Test redirect latency
    local count=100
    local total_time=0
    local times=()

    print_info "Testing redirect latency ($count requests)..."

    for i in $(seq 1 $count); do
        local time_ms=$(curl -s -o /dev/null -w "%{time_total}" "${BASE_URL}/${short_code}" --max-redirs 0 2>/dev/null | awk '{printf "%.3f", $1 * 1000}')
        times+=("$time_ms")
        total_time=$(echo "$total_time + $time_ms" | bc)
    done

    # Sort times
    IFS=$'\n' sorted=($(sort -n <<<"${times[*]}")); unset IFS

    local avg=$(echo "scale=3; $total_time / $count" | bc)
    local min=${sorted[0]}
    local max=${sorted[$((count-1))]}
    local p50=${sorted[$((count/2))]}
    local p95=${sorted[$((count*95/100))]}
    local p99=${sorted[$((count*99/100))]}

    echo ""
    echo -e "  ${BOLD}Min:${NC}     ${min}ms"
    echo -e "  ${BOLD}Avg:${NC}     ${avg}ms"
    echo -e "  ${BOLD}P50:${NC}     ${p50}ms"
    echo -e "  ${BOLD}P95:${NC}     ${p95}ms"
    echo -e "  ${BOLD}P99:${NC}     ${p99}ms"
    echo -e "  ${BOLD}Max:${NC}     ${max}ms"

    if (( $(echo "$p99 < 50" | bc -l) )); then
        print_success "Redirect P99 latency is under 50ms"
    else
        print_error "Redirect P99 latency is high: ${p99}ms"
    fi

    # Store short_code for concurrent tests
    echo "$short_code"
}

# Test concurrent requests
test_concurrency() {
    local short_code=$1

    print_section "Concurrency Test"

    print_info "Parameters:"
    echo -e "  Concurrency:   ${CONCURRENCY}"
    echo -e "  Total Requests: ${REQUESTS}"
    echo ""

    # Create temporary files for results
    local results_file=$(mktemp)
    local errors_file=$(mktemp)
    local latency_file=$(mktemp)

    trap "rm -f $results_file $errors_file $latency_file" EXIT

    print_info "Running concurrent load test..."

    local start_time=$(date +%s.%N)

    # Use xargs for parallel execution
    seq 1 $REQUESTS | xargs -P $CONCURRENCY -I {} bash -c "
        start=\$(date +%s.%N)
        status=\$(curl -s -o /dev/null -w '%{http_code}' '${BASE_URL}/${short_code}' --max-redirs 0 2>/dev/null)
        end=\$(date +%s.%N)
        latency=\$(echo \"\$end - \$start\" | bc | awk '{printf \"%.3f\", \$1 * 1000}')

        if [ \"\$status\" = \"302\" ] || [ \"\$status\" = \"301\" ]; then
            echo \"1\" >> $results_file
            echo \"\$latency\" >> $latency_file
        else
            echo \"0\" >> $errors_file
        fi
    " 2>/dev/null

    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc)

    # Count results
    local success_count=$(wc -l < "$results_file" 2>/dev/null | tr -d ' ')
    local error_count=$(wc -l < "$errors_file" 2>/dev/null | tr -d ' ')
    local total=$((success_count + error_count))

    if [ "$total" -eq 0 ]; then
        total=1  # Avoid division by zero
    fi

    local success_rate=$(echo "scale=2; $success_count * 100 / $total" | bc)
    local rps=$(echo "scale=2; $success_count / $duration" | bc)

    # Calculate latency percentiles
    if [ -s "$latency_file" ]; then
        local latencies=($(sort -n "$latency_file"))
        local lat_count=${#latencies[@]}

        if [ "$lat_count" -gt 0 ]; then
            local lat_min=${latencies[0]}
            local lat_max=${latencies[$((lat_count-1))]}
            local lat_p50=${latencies[$((lat_count/2))]}
            local lat_p95=${latencies[$((lat_count*95/100))]}
            local lat_p99=${latencies[$((lat_count*99/100))]}

            local lat_total=0
            for lat in "${latencies[@]}"; do
                lat_total=$(echo "$lat_total + $lat" | bc)
            done
            local lat_avg=$(echo "scale=3; $lat_total / $lat_count" | bc)
        fi
    fi

    echo ""
    echo -e "${BOLD}Results:${NC}"
    echo -e "  Duration:      ${duration}s"
    echo -e "  Successful:    ${success_count} (${success_rate}%)"
    echo -e "  Failed:        ${error_count}"
    echo -e "  RPS:           ${rps} req/sec"
    echo ""
    echo -e "${BOLD}Latency:${NC}"
    echo -e "  Min:           ${lat_min:-N/A}ms"
    echo -e "  Avg:           ${lat_avg:-N/A}ms"
    echo -e "  P50:           ${lat_p50:-N/A}ms"
    echo -e "  P95:           ${lat_p95:-N/A}ms"
    echo -e "  P99:           ${lat_p99:-N/A}ms"
    echo -e "  Max:           ${lat_max:-N/A}ms"

    # Evaluate results
    echo ""
    if (( $(echo "$success_rate >= 99" | bc -l) )); then
        print_success "Success rate is ${success_rate}% (>= 99%)"
    else
        print_error "Success rate is ${success_rate}% (< 99%)"
    fi

    if [ -n "$lat_p99" ] && (( $(echo "$lat_p99 < 100" | bc -l) )); then
        print_success "P99 latency is ${lat_p99}ms (< 100ms)"
    elif [ -n "$lat_p99" ]; then
        print_error "P99 latency is ${lat_p99}ms (>= 100ms)"
    fi
}

# Test URL shortening throughput
test_shorten_throughput() {
    print_section "URL Shortening Throughput Test"

    local count=50
    local total_time=0
    local success=0

    print_info "Testing URL creation throughput ($count requests)..."

    local start_time=$(date +%s.%N)

    for i in $(seq 1 $count); do
        local response=$(curl -s -X POST "${BASE_URL}/api/v1/shorten" \
            -H "Content-Type: application/json" \
            -d "{\"url\":\"https://example.com/throughput/${i}\"}" \
            -w "\n%{http_code}" 2>/dev/null)

        local status=$(echo "$response" | tail -n1)
        if [ "$status" = "201" ]; then
            ((success++))
        fi
    done

    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc)
    local rps=$(echo "scale=2; $success / $duration" | bc)

    echo ""
    echo -e "  ${BOLD}Requests:${NC}    $count"
    echo -e "  ${BOLD}Successful:${NC}  $success"
    echo -e "  ${BOLD}Duration:${NC}    ${duration}s"
    echo -e "  ${BOLD}RPS:${NC}         ${rps} req/sec"

    if [ "$success" -eq "$count" ]; then
        print_success "All URL creation requests succeeded"
    else
        print_error "Some URL creation requests failed"
    fi
}

# Main execution
main() {
    print_header "GoURL Load Testing Suite"

    print_info "Target:      ${BASE_URL}"
    print_info "Mode:        ${MODE}"
    print_info "Concurrency: ${CONCURRENCY}"
    print_info "Requests:    ${REQUESTS}"

    check_server

    test_health_latency

    local short_code=$(test_redirect_latency | tail -1)

    if [ -n "$short_code" ] && [ ${#short_code} -gt 0 ]; then
        test_concurrency "$short_code"
    fi

    test_shorten_throughput

    print_header "Load Test Complete"
}

main
