# Migration Path: MVP → Production

> **Incremental evolution strategy with zero downtime**

## 📖 Document Overview

**Purpose:** Step-by-step migration from Phase 1 MVP to full production architecture

**In this document:**
- [Migration Phases](#migration-phases) - 8-phase overview diagram
- [Phase 2: Kafka Introduction](#phase-2-kafka-introduction-dual-write) - Add Kafka alongside Redis
- [Phase 3: Kafka Primary](#phase-3-kafka-primary-dual-read) - Switch to Kafka
- [Phase 4: CQRS Projections](#phase-4-cqrs-projections) - Add read models
- [Phase 5: WASM Optimizer](#phase-7-wasm-optimizer) - Compile optimizer
- [Phase 6-8: gRPC, Protobuf, Execution Envs](#phase-6-grpc-migration) - Final phases
- [Migration Complexity](#migration-complexity-matrix) - Risk and effort
- [Rollback Strategy](#rollback-strategy) - Feature flags and safety
- [Timeline](#timeline-summary) - 12-month roadmap

---

## Overview

This document outlines the step-by-step migration from the Phase 1 MVP to the full production vision. Every step is designed for **incremental deployment**, **backward compatibility**, and **rollback safety**.

**Timeline:** 6-12 months for full migration, deployed incrementally

**Philosophy:** No throwaway code - every MVP decision was made with migration in mind

---

## Migration Phases

```
Phase 1: MVP (✅ COMPLETE)
  └─ Redis Streams + HTTP REST + JSON + Direct Postgres
       ↓
Phase 2: Kafka Introduction (Dual-Write)
  └─ Redis + Kafka (both) + HTTP REST + JSON
       ↓
Phase 3: Kafka Primary (Dual-Read)
  └─ Kafka (primary) + Redis (backup) + HTTP REST + JSON
       ↓
Phase 4: CQRS Projections
  └─ Kafka + Projections Service + Redis
       ↓
Phase 5: WASM Optimizer
  └─ Kafka + WASM graph optimization
       ↓
Phase 6: gRPC Migration
  └─ Kafka + WASM + gRPC (alongside HTTP)
       ↓
Phase 7: Protobuf Adoption
  └─ Kafka + WASM + gRPC + Protobuf
       ↓
Phase 8: Customer Execution Environments
  └─ Full production architecture (K8s, Lambda, customer infra)
```

---

## Phase 2: Kafka Introduction (Dual-Write)

**Goal:** Add Kafka without disrupting existing Redis-based system

**Duration:** 2-4 weeks

### Changes

1. **Add Kafka cluster**
   ```bash
   # Deploy Kafka (3 brokers)
   kafka-topics --create --topic workflow.events --partitions 64 --replication-factor 3
   kafka-topics --create --topic node.jobs.high --partitions 64 --replication-factor 3
   kafka-topics --create --topic node.results --partitions 64 --replication-factor 3
   ```

2. **Dual-write pattern**
   ```go
   // cmd/orchestrator/service/event_publisher.go
   func (p *EventPublisher) Publish(event Event) error {
       // Write to Redis (existing)
       p.redis.Publish("run:" + event.RunID, event)

       // ALSO write to Kafka (new)
       p.kafka.Publish("workflow.events", event)

       return nil
   }
   ```

3. **Kafka consumer (shadow mode)**
   ```go
   // New service: kafka-consumer (validate only, don't act)
   func (c *KafkaConsumer) Start() {
       for msg := range c.kafkaReader.ReadMessages() {
           var event Event
           json.Unmarshal(msg.Value, &event)

           // Validate against Redis version
           redisEvent := c.redis.Get("event:" + event.EventID)
           if !equal(event, redisEvent) {
               log.Warn("kafka/redis mismatch", "event_id", event.EventID)
           }
       }
   }
   ```

### Validation

- [ ] All events written to both Redis and Kafka
- [ ] Kafka consumer validates consistency
- [ ] No production traffic routed through Kafka yet
- [ ] Metrics: kafka_write_success_rate = 100%

### Rollback

```bash
# Disable Kafka writes (feature flag)
ENABLE_KAFKA_WRITE=false

# Or remove Kafka publisher
```

---

## Phase 3: Kafka Primary (Dual-Read)

**Goal:** Switch reads to Kafka, keep Redis as backup

**Duration:** 2-4 weeks

### Changes

1. **Route workers to Kafka**
   ```go
   // cmd/workflow-runner/coordinator/completion_handler.go
   func (c *Coordinator) Start() {
       if enableKafka {
           // Read from Kafka
           c.startKafkaConsumer()
       } else {
           // Fallback to Redis
           c.startRedisConsumer()
       }
   }
   ```

2. **Gradual rollout** (feature flag per run)
   ```go
   func (c *Coordinator) chooseBackend(runID string) Backend {
       // 10% of traffic to Kafka
       if hash(runID) % 100 < 10 {
           return KafkaBackend
       }
       return RedisBackend
   }
   ```

3. **Increase Kafka traffic**
   ```
   Week 1: 10% Kafka, 90% Redis
   Week 2: 50% Kafka, 50% Redis
   Week 3: 90% Kafka, 10% Redis
   Week 4: 100% Kafka, 0% Redis (Redis backup only)
   ```

### Validation

- [ ] Kafka throughput matches expectations
- [ ] No increase in error rates
- [ ] Latency comparable to Redis
- [ ] Metrics: kafka_read_success_rate = 100%

### Rollback

```bash
# Flip feature flag back to Redis
KAFKA_TRAFFIC_PERCENT=0

# Traffic instantly routes back to Redis
```

---

## Phase 4: CQRS Projections

**Goal:** Add read models for fast queries

**Duration:** 3-4 weeks

### Changes

1. **Add projection tables**
   ```sql
   -- Read model (materialized view)
   CREATE TABLE runs_read (
       run_id UUID PRIMARY KEY,
       status TEXT,
       started_at TIMESTAMPTZ,
       ended_at TIMESTAMPTZ,
       active_nodes INT,
       cost_cents BIGINT,
       last_event_ts TIMESTAMPTZ,
       version TEXT
   );

   CREATE TABLE states_read (
       run_id UUID,
       node_id TEXT,
       status TEXT,
       outputs_ref TEXT,
       PRIMARY KEY (run_id, node_id)
   );

   -- Indexes for fast lookups
   CREATE INDEX idx_runs_read_status ON runs_read(status);
   CREATE INDEX idx_runs_read_started ON runs_read(started_at DESC);
   ```

2. **Projection service**
   ```go
   // cmd/projections/worker.go
   func (w *ProjectionWorker) ProcessEvent(event Event) {
       switch event.Type {
       case "NodeCompleted":
           // Update states_read
           w.db.Exec(`
               UPDATE states_read
               SET status = 'completed', ended_at = $1
               WHERE run_id = $2 AND node_id = $3
           `, event.Timestamp, event.RunID, event.NodeID)

           // Update runs_read
           w.db.Exec(`
               UPDATE runs_read
               SET active_nodes = active_nodes - 1
               WHERE run_id = $1
           `, event.RunID)

       case "RunCompleted":
           w.db.Exec(`
               UPDATE runs_read
               SET status = 'completed', ended_at = $1
               WHERE run_id = $2
           `, event.Timestamp, event.RunID)
       }
   }
   ```

3. **Update API to use projections**
   ```go
   // cmd/orchestrator/handlers/run.go
   func (h *RunHandler) GetRun(c echo.Context) error {
       runID := c.Param("id")

       // Read from projection (fast!)
       run := h.db.QueryRow(`
           SELECT * FROM runs_read WHERE run_id = $1
       `, runID)

       return c.JSON(200, run)
   }
   ```

### Validation

- [ ] Projections eventually consistent (< 1s lag)
- [ ] Read queries 10x faster
- [ ] Write throughput unchanged
- [ ] Metrics: projection_lag_seconds < 1

### Rollback

```bash
# Route reads back to main tables
USE_PROJECTIONS=false

# Projection service can run in background (no impact)
```

---

## Phase 5: gRPC Migration

**Goal:** Add gRPC alongside HTTP for low-latency calls

**Duration:** 3-4 weeks

### Changes

1. **Define protobuf schemas**
   ```protobuf
   // api/proto/workflow.proto
   syntax = "proto3";

   message StartRunRequest {
       string workflow_tag = 1;
       map<string, string> inputs = 2;
   }

   message StartRunResponse {
       string run_id = 1;
       string status = 2;
   }

   service WorkflowService {
       rpc StartRun(StartRunRequest) returns (StartRunResponse);
       rpc GetRun(GetRunRequest) returns (GetRunResponse);
   }
   ```

2. **Implement gRPC server**
   ```go
   // cmd/orchestrator/grpc_server.go
   type WorkflowServer struct {
       service *service.WorkflowService
   }

   func (s *WorkflowServer) StartRun(ctx context.Context, req *pb.StartRunRequest) (*pb.StartRunResponse, error) {
       // Call existing service
       run, err := s.service.StartRun(ctx, req)
       return &pb.StartRunResponse{
           RunId:  run.ID,
           Status: run.Status,
       }, err
   }
   ```

3. **Run both HTTP and gRPC**
   ```go
   // cmd/orchestrator/main.go
   func main() {
       // HTTP server (existing)
       go http.ListenAndServe(":8080", httpRouter)

       // gRPC server (new)
       grpcServer := grpc.NewServer()
       pb.RegisterWorkflowServiceServer(grpcServer, &WorkflowServer{...})
       lis, _ := net.Listen("tcp", ":9090")
       grpcServer.Serve(lis)
   }
   ```

4. **Gradually migrate internal calls to gRPC**
   ```
   Week 1: HTTP only (baseline)
   Week 2: 10% internal calls use gRPC
   Week 3: 50% internal calls use gRPC
   Week 4: 100% internal calls use gRPC
   Week 5+: External clients can use gRPC (opt-in)
   ```

### Validation

- [ ] gRPC latency < HTTP latency
- [ ] Both protocols work simultaneously
- [ ] Metrics: grpc_success_rate = 100%

### Rollback

```bash
# Stop gRPC server
ENABLE_GRPC=false

# All traffic routes through HTTP (zero impact)
```

---

## Phase 6: Protobuf Adoption

**Goal:** Use binary protobuf instead of JSON for efficiency

**Duration:** 2-3 weeks

### Changes

1. **Dual serialization**
   ```go
   // Serialize to both JSON and protobuf
   func (e *Event) Marshal() ([]byte, error) {
       if useProtobuf {
           return proto.Marshal(e)
       }
       return json.Marshal(e)
   }
   ```

2. **Content-Type negotiation**
   ```go
   // Accept both JSON and protobuf
   func (h *Handler) StartRun(c echo.Context) error {
       contentType := c.Request().Header.Get("Content-Type")

       var req StartRunRequest
       if contentType == "application/protobuf" {
           proto.Unmarshal(c.Request().Body, &req)
       } else {
           json.Unmarshal(c.Request().Body, &req)
       }

       // ...
   }
   ```

3. **Gradual migration**
   ```
   Week 1: Accept both JSON and protobuf (default JSON)
   Week 2: 50% clients migrate to protobuf
   Week 3: 90% clients migrate to protobuf
   Week 4: Deprecate JSON (keep for backward compat)
   ```

### Validation

- [ ] Protobuf payload 50-70% smaller
- [ ] Serialization 2-3x faster
- [ ] Both formats work simultaneously

---

## Phase 7: WASM Optimizer

**Goal:** Compile Rust optimizer to WASM and integrate

**Duration:** 4-6 weeks

### Changes

1. **Implement optimizers in Rust**
   ```rust
   // crates/dag-optimizer/src/optimizers/http_coalescer.rs
   pub fn coalesce_http_nodes(ir: &mut IR) -> Vec<OptimizedPatch> {
       let mut patches = Vec::new();

       // Find sequential HTTP nodes with same host
       for window in ir.nodes.windows(2) {
           if can_coalesce(&window[0], &window[1]) {
               patches.push(create_batch_node(window));
           }
       }

       patches
   }
   ```

2. **Compile to WASM**
   ```bash
   # Cargo.toml
   [lib]
   crate-type = ["cdylib"]

   # Build
   cargo build --target wasm32-unknown-unknown --release
   wasm-opt -O3 -o optimizer.wasm target/wasm32-unknown-unknown/release/optimizer.wasm
   ```

3. **Integrate with Go**
   ```go
   // cmd/optimizer/wasm_runtime.go
   import "github.com/wasmerio/wasmer-go/wasmer"

   func (o *Optimizer) RunOptimizationPass(ir *IR, passName string) (*OptimizedPatch, error) {
       // Load WASM module
       module, _ := wasmer.NewModule(store, wasmBytes)
       instance, _ := wasmer.NewInstance(module, nil)

       // Call optimizer function
       optimize := instance.Exports["optimize"]
       result, _ := optimize(ir.ToBytes())

       return ParsePatch(result)
   }
   ```

4. **Deploy optimizer as service**
   ```go
   // cmd/optimizer/main.go
   func main() {
       // Subscribe to Kafka: optimization.requests
       for req := range kafkaReader.ReadMessages() {
           ir := parseIR(req.Value)
           patch := runOptimizations(ir)
           kafka.Publish("optimization.patches", patch)
       }
   }
   ```

### Validation

- [ ] WASM optimizer produces valid patches
- [ ] Performance improvement measured
- [ ] Can be updated without service restart

---

## Phase 8: Customer Execution Environments

**Goal:** Support agent execution in customer-provided infrastructure

**Duration:** 2-3 weeks

### Changes

1. **Add execution environment abstraction**
   ```go
   // pkg/agent/executor.go
   type ExecutionEnvironment interface {
       Submit(job AgentJob) (string, error)
       GetStatus(jobID string) (JobStatus, error)
       GetResult(jobID string) (AgentResult, error)
   }

   // Implementations:
   // - K8sExecutor (Kubernetes Jobs)
   // - LambdaExecutor (AWS Lambda)
   // - LocalExecutor (local processes - MVP)
   // - CustomExecutor (customer-provided)
   ```

2. **K8s executor implementation**
   ```go
   func (e *K8sExecutor) Submit(job AgentJob) (string, error) {
       jobSpec := e.generateJobSpec(job)
       result := e.k8sClient.BatchV1().Jobs("workflows").Create(ctx, jobSpec)
       return result.Name, nil
   }
   ```

3. **Lambda executor**
   ```go
   func (e *LambdaExecutor) Submit(job AgentJob) (string, error) {
       payload, _ := json.Marshal(job)
       result := e.lambdaClient.Invoke(&lambda.InvokeInput{
           FunctionName: aws.String("agent-runner"),
           Payload:      payload,
       })
       return *result.RequestId, nil
   }
   ```

4. **Configuration**
   ```yaml
   # config.yaml
   agent_execution:
     environment: kubernetes  # or: lambda, local, custom
     kubernetes:
       namespace: workflows
       image: acme/agent-runner:v1.2
       resources:
         memory: 2Gi
         cpu: 1000m
     lambda:
       function_name: agent-runner
       runtime: python3.11
     custom:
       api_endpoint: https://customer-agents.example.com
   ```

### Validation

- [ ] Agent jobs run in isolated environments
- [ ] Results returned correctly
- [ ] Resource limits enforced
- [ ] Customer can deploy to their infra

---

## Migration Complexity Matrix

| Component | Complexity | Risk | Migration Time |
|-----------|-----------|------|----------------|
| **Redis → Kafka** | Medium | Low | 4-8 weeks |
| **Direct Postgres → CQRS** | High | Medium | 3-4 weeks |
| **HTTP → gRPC** | Low | Low | 3-4 weeks |
| **JSON → Protobuf** | Low | Low | 2-3 weeks |
| **Stub → WASM Optimizer** | High | Low | 4-6 weeks |
| **WebSocket → WebTransport** | Medium | Medium | 4-6 weeks |
| **Local → Customer exec envs** | Medium | Low | 3-4 weeks |

---

## Rollback Strategy

Every phase must have a rollback plan:

### 1. Feature Flags

```go
// common/config/features.go
type FeatureFlags struct {
    EnableKafka       bool `env:"ENABLE_KAFKA" default:"false"`
    EnableProjections bool `env:"ENABLE_PROJECTIONS" default:"false"`
    EnableGRPC        bool `env:"ENABLE_GRPC" default:"false"`
    EnableProtobuf    bool `env:"ENABLE_PROTOBUF" default:"false"`
    EnableOPA         bool `env:"ENABLE_OPA" default:"false"`
}
```

### 2. Gradual Rollout

```go
// Route percentage of traffic to new backend
func (r *Router) route(req Request) Backend {
    if hash(req.ID) % 100 < r.newBackendPercent {
        return r.newBackend
    }
    return r.oldBackend
}
```

### 3. Rollback Triggers

Automatic rollback if:
- Error rate > 5% (baseline: <1%)
- P95 latency > 2x baseline
- Kafka lag > 10 seconds
- Projection lag > 5 seconds

```go
// Monitor and auto-rollback
func (m *Monitor) checkHealth() {
    if m.errorRate() > 0.05 {
        log.Error("High error rate, rolling back")
        m.featureFlags.EnableKafka = false
        m.applyConfig()
    }
}
```

---

## Migration Validation Checklist

Before moving to next phase:

- [ ] All tests pass
- [ ] Load testing shows no regression
- [ ] Metrics stable for 1 week
- [ ] Error rates < baseline
- [ ] Latency P95 < baseline
- [ ] Rollback tested successfully
- [ ] Monitoring dashboards updated
- [ ] Documentation updated

---

## Timeline Summary

```
Month 1-2:   Phase 2 (Kafka dual-write) + Phase 3 (Kafka primary)
Month 3:     Phase 4 (CQRS projections - optional)
Month 4-5:   Phase 5 (WASM optimizer)
Month 6:     Phase 6 (gRPC migration)
Month 7:     Phase 7 (Protobuf adoption)
Month 8-9:   Phase 8 (Customer execution environments)
Month 10:    WebTransport, eBPF observability
Month 11-12: Performance tuning, final optimization

Total: 12 months
```

---

## Why This Migration Works

1. **Incremental:** Each phase is small, testable, reversible
2. **Dual-mode:** Old and new systems run in parallel
3. **Gradual rollout:** Percentage-based traffic shifting
4. **Zero downtime:** No service interruption
5. **Backward compatible:** Old clients still work
6. **Rollback safe:** Feature flags + monitoring
7. **Validated at each step:** Metrics confirm success before next phase

---

## References

**Current State:**
- [CURRENT.md](./CURRENT.md) - Phase 1 implementation

**Target State:**
- [VISION.md](./VISION.md) - Production architecture

**Operations:**
- [../operations/SCALABILITY.md](../operations/SCALABILITY.md) - Performance tuning

---

**All MVP decisions were made with this migration path in mind. No throwaway code!**
