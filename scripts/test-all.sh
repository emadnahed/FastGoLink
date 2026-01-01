#!/usr/bin/env bash
#
# Comprehensive Test Runner for GoURL
# Runs all tests (unit, integration, e2e) with coverage reporting
#
# Usage:
#   ./scripts/test-all.sh              # Run all tests without Docker
#   ./scripts/test-all.sh --docker     # Run all tests with Docker services
#   ./scripts/test-all.sh --help       # Show help
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Configuration
COVERAGE_FILE="coverage.out"
COVERAGE_HTML="coverage.html"
COVERAGE_THRESHOLD=0  # Set to desired minimum coverage percentage (0 = no threshold)

# Counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
SKIPPED_TESTS=0

# Flags
USE_DOCKER=false
VERBOSE=false
SHOW_COVERAGE_HTML=false

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Change to project root
cd "$PROJECT_ROOT"

# Print functions
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

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_info() {
    echo -e "${CYAN}ℹ${NC} $1"
}

# Help function
show_help() {
    echo "GoURL Comprehensive Test Runner"
    echo ""
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --docker      Start Docker services (PostgreSQL, Redis) for full integration tests"
    echo "  --verbose     Show verbose test output"
    echo "  --open        Open HTML coverage report after tests complete"
    echo "  --help        Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                    Run all tests (unit tests + mocked integration/e2e)"
    echo "  $0 --docker           Run all tests with real PostgreSQL and Redis"
    echo "  $0 --docker --open    Run tests with Docker and open coverage report"
    echo ""
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --docker)
            USE_DOCKER=true
            shift
            ;;
        --verbose)
            VERBOSE=true
            shift
            ;;
        --open)
            SHOW_COVERAGE_HTML=true
            shift
            ;;
        --help|-h)
            show_help
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            ;;
    esac
done

# Cleanup function
cleanup() {
    if [ "$USE_DOCKER" = true ]; then
        print_section "Cleaning up Docker services..."
        docker-compose down 2>/dev/null || true
    fi
}

# Set trap for cleanup on exit
trap cleanup EXIT

# Start Docker services if requested
start_docker_services() {
    print_section "Starting Docker Services"

    # Check if docker-compose is available
    if ! command -v docker-compose &> /dev/null; then
        print_error "docker-compose not found. Please install Docker Compose."
        exit 1
    fi

    # Start services
    print_info "Starting PostgreSQL and Redis..."
    docker-compose up -d postgres redis

    # Wait for services to be healthy
    print_info "Waiting for services to be ready..."

    local max_attempts=30
    local attempt=0

    # Wait for PostgreSQL
    while [ $attempt -lt $max_attempts ]; do
        if docker-compose exec -T postgres pg_isready -U postgres &>/dev/null; then
            print_success "PostgreSQL is ready"
            break
        fi
        attempt=$((attempt + 1))
        sleep 1
    done

    if [ $attempt -eq $max_attempts ]; then
        print_error "PostgreSQL failed to start within timeout"
        exit 1
    fi

    # Wait for Redis
    attempt=0
    while [ $attempt -lt $max_attempts ]; do
        if docker-compose exec -T redis redis-cli ping &>/dev/null; then
            print_success "Redis is ready"
            break
        fi
        attempt=$((attempt + 1))
        sleep 1
    done

    if [ $attempt -eq $max_attempts ]; then
        print_error "Redis failed to start within timeout"
        exit 1
    fi

    echo ""
    print_success "All Docker services are running"
}

# Run tests for a specific package/directory
run_test_suite() {
    local suite_name=$1
    local test_path=$2
    local env_vars=$3

    print_section "Running $suite_name"

    local test_cmd="go test -v -race -count=1"

    if [ -n "$env_vars" ]; then
        test_cmd="$env_vars $test_cmd"
    fi

    test_cmd="$test_cmd $test_path"

    print_info "Command: $test_cmd"
    echo ""

    # Run tests and capture output
    local start_time=$(date +%s)

    if eval "$test_cmd" 2>&1; then
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        print_success "$suite_name completed in ${duration}s"
        return 0
    else
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        print_error "$suite_name failed after ${duration}s"
        return 1
    fi
}

# Run all tests with coverage
run_all_tests_with_coverage() {
    print_section "Running All Tests with Coverage"

    local env_vars=""
    if [ "$USE_DOCKER" = true ]; then
        env_vars="TEST_POSTGRES=true TEST_REDIS=true"
    fi

    local test_cmd="go test -v -race -coverprofile=$COVERAGE_FILE -covermode=atomic ./..."

    if [ -n "$env_vars" ]; then
        test_cmd="$env_vars $test_cmd"
    fi

    print_info "Command: $test_cmd"
    echo ""

    local start_time=$(date +%s)

    if eval "$test_cmd" 2>&1; then
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        print_success "All tests completed in ${duration}s"
        return 0
    else
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        print_error "Some tests failed after ${duration}s"
        return 1
    fi
}

# Generate and display coverage report
show_coverage_report() {
    print_section "Coverage Report"

    if [ ! -f "$COVERAGE_FILE" ]; then
        print_warning "Coverage file not found"
        return
    fi

    # Generate HTML report
    if go tool cover -html="$COVERAGE_FILE" -o "$COVERAGE_HTML" 2>/dev/null; then
        print_success "HTML coverage report generated: $COVERAGE_HTML"
    else
        print_warning "Could not generate HTML coverage report"
    fi

    echo ""
    print_info "Coverage by package:"
    echo ""

    # Show coverage per package - use a temp file to avoid subshell issues
    local coverage_output
    coverage_output=$(go tool cover -func="$COVERAGE_FILE" 2>/dev/null) || {
        print_warning "Could not generate coverage function report"
        return
    }

    local total_coverage=""

    while IFS= read -r line; do
        if [[ $line == *"total:"* ]]; then
            total_coverage=$(echo "$line" | awk '{print $NF}' | tr -d '%')
            echo ""
            echo -e "${BOLD}═══════════════════════════════════════════════════════════════${NC}"
            # Use awk for floating point comparison
            if awk "BEGIN {exit !($total_coverage >= 80)}"; then
                echo -e "${GREEN}${BOLD}  TOTAL COVERAGE: ${total_coverage}%${NC}"
            elif awk "BEGIN {exit !($total_coverage >= 60)}"; then
                echo -e "${YELLOW}${BOLD}  TOTAL COVERAGE: ${total_coverage}%${NC}"
            else
                echo -e "${RED}${BOLD}  TOTAL COVERAGE: ${total_coverage}%${NC}"
            fi
            echo -e "${BOLD}═══════════════════════════════════════════════════════════════${NC}"
        else
            echo "  $line"
        fi
    done <<< "$coverage_output"

    # Check coverage threshold
    if [ "$COVERAGE_THRESHOLD" -gt 0 ] && [ -n "$total_coverage" ]; then
        if awk "BEGIN {exit !($total_coverage < $COVERAGE_THRESHOLD)}"; then
            echo ""
            print_error "Coverage ${total_coverage}% is below threshold ${COVERAGE_THRESHOLD}%"
            return 1
        fi
    fi

    # Open HTML report if requested
    if [ "$SHOW_COVERAGE_HTML" = true ]; then
        echo ""
        print_info "Opening coverage report in browser..."
        if command -v open &> /dev/null; then
            open "$COVERAGE_HTML"
        elif command -v xdg-open &> /dev/null; then
            xdg-open "$COVERAGE_HTML"
        else
            print_warning "Could not open browser. View report at: $COVERAGE_HTML"
        fi
    fi
}

# Count and display test summary
show_test_summary() {
    local exit_code=$1

    print_header "TEST SUMMARY"

    # Parse coverage file for stats
    if [ -f "$COVERAGE_FILE" ]; then
        local total_coverage
        total_coverage=$(go tool cover -func="$COVERAGE_FILE" 2>/dev/null | grep total | awk '{print $NF}') || total_coverage="N/A"

        if [ -n "$total_coverage" ]; then
            echo -e "  ${BOLD}Total Coverage:${NC}  $total_coverage"
        fi
    fi

    echo ""

    if [ "$exit_code" -eq 0 ]; then
        echo -e "  ${GREEN}${BOLD}STATUS: ALL TESTS PASSED${NC}"
    else
        echo -e "  ${RED}${BOLD}STATUS: SOME TESTS FAILED${NC}"
    fi

    echo ""
    if [ -f "$COVERAGE_HTML" ]; then
        echo -e "  ${CYAN}Coverage Report:${NC}  $COVERAGE_HTML"
    fi
    echo -e "  ${CYAN}Coverage Data:${NC}    $COVERAGE_FILE"
    echo ""
}

# Main execution
main() {
    print_header "GoURL Comprehensive Test Suite"

    print_info "Project: $(basename "$PROJECT_ROOT")"
    print_info "Date: $(date)"
    print_info "Go Version: $(go version | awk '{print $3}')"

    if [ "$USE_DOCKER" = true ]; then
        print_info "Mode: Full Integration (with Docker)"
    else
        print_info "Mode: Standard (without Docker)"
    fi

    echo ""

    # Start Docker if requested
    if [ "$USE_DOCKER" = true ]; then
        start_docker_services
    fi

    # Track overall status
    local test_status=0

    # Run all tests with coverage
    if ! run_all_tests_with_coverage; then
        test_status=1
    fi

    # Show coverage report (don't affect test status)
    show_coverage_report || true

    # Show final summary
    show_test_summary $test_status

    return $test_status
}

# Run main
main
exit $?
