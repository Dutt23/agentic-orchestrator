# Performance Test Usage Guide

## Setup

### 1. Set Test Token
```bash
# In .env
PERF_TEST_TOKEN=my-secret-token
```

### 2. Register Test Routes
**Add to `cmd/orchestrator/main.go`:**
```go
// After other route registrations
routes.RegisterTestRoutes(e, container)
```

### 3. Start Services
```bash
# Without mover (baseline)
USE_MOVER=false ./scripts/start-all.sh

# With mover (optimized)
USE_MOVER=true ./scripts/start-all.sh
```

---

## Running Tests

### Create Test Workflow
```bash
curl -X POST http://localhost:8081/api/v1/test/create-workflow \
  -H "X-Test-Token: my-secret-token" \
  -d '{"run_id":"test-perf-wf-123","node_count":10}'
```

### Run Performance Test

**Quick test (1000 calls):**
```bash
PERF_TEST_TOKEN=my-secret-token \
PERF_NUM_CALLS=1000 \
PERF_CONCURRENCY=10 \
  go test -v ./perf_tests/workflows/ -run=TestFetchWorkflowsConcurrent
```

**Full test (100,000 calls):**
```bash
PERF_TEST_TOKEN=my-secret-token \
PERF_NUM_CALLS=100000 \
PERF_CONCURRENCY=10 \
  go test -v ./perf_tests/workflows/ -run=TestFetchWorkflowsConcurrent
```

### Compare With/Without Mover

```bash
# Terminal 1: Without mover
USE_MOVER=false cmd/orchestrator/start.sh

# Terminal 2: Run test
PERF_TEST_TOKEN=my-secret-token \
USE_MOVER=false \
PERF_NUM_CALLS=100000 \
  go test -v ./perf_tests/workflows/ -run=TestFetchWorkflowsConcurrent

# Terminal 1: Restart with mover
USE_MOVER=true ./scripts/start-mover.sh orchestrator
USE_MOVER=true cmd/orchestrator/start.sh

# Terminal 2: Run test again
PERF_TEST_TOKEN=my-secret-token \
USE_MOVER=true \
PERF_NUM_CALLS=100000 \
  go test -v ./perf_tests/workflows/ -run=TestFetchWorkflowsConcurrent
```

---

## Test Endpoints

**All require `X-Test-Token` header!**

### GET /api/v1/test/fetch-workflow/{run_id}
Fetches workflow IR (what workflow-runner does)

### GET /api/v1/test/fetch-cas/{cas_id}
Fetches from CAS

### POST /api/v1/test/create-workflow
Creates test workflow
Body: `{"run_id":"test-123","node_count":10}`

---

## Security

**Protected by:**
- X-Test-Token header (must match PERF_TEST_TOKEN)
- Middleware rejects requests without valid token
- Safe to deploy - can't be called without token

**To disable in production:**
- Don't register test routes
- Or set PERF_TEST_TOKEN to something secret
