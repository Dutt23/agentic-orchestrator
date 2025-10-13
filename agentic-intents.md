# Agentic Service & Intents

> The Agentic Service only produces **signed JSON intents** and **AIR graph patches**. It never executes code. All actions are delegated to the Command Router and downstream services after policy verification.

---

## 1. Design Goals

* **Safety**: No code execution or arbitrary I/O; JSON only.
* **Determinism**: Typed schemas; allowlisted intents; TTL + idempotency.
* **Auditability**: Signed envelopes; full event trail; actor attribution.
* **Extensibility**: New intents via schema; no Agentic core changes.

---

## 2. Intent Envelope (standard)

Every agent output is wrapped in a signed envelope.

```json
{
  "version": "1.0",
  "intent": {
    "type": "logs.stream",
    "args": {"run_id": "7f3e…", "filter": "errors", "node_id": "triage"}
  },
  "actor": {"user_id": "u_123", "tenant": "acme", "roles": ["dev"]},
  "constraints": {
    "ttl_sec": 120,
    "idempotency_key": "sha256(run_id|args)",
    "capabilities": ["logs:read"]
  },
  "trace_id": "abcd-1234",
  "sig": "ed25519:base64…"
}
```

### 2.1 Signature & Validation

* **Signature**: JWS/Ed25519 over canonical JSON (JCS). Key managed by Agentic Service; rotated and published.
* **Router checks**: signature → TTL → idempotency → RBAC → OPA policy → dispatch.

---

## 3. Intent Catalog (MVP)

### 3.1 workflow.start

```json
{
  "type": "workflow.start",
  "args": {
    "workflow_id": "lead_flow",
    "inputs": {"file": "cas://sha256:…", "priority": "high"}
  }
}
```

* **Dispatch**: Orchestrator → StartRun
* **Policy**: user must have `runs:start` on workflow

### 3.2 run.replay

```json
{
  "type": "run.replay",
  "args": {"run_id": "7f…", "from_node": "parse", "mode": "freeze"}
}
```

* **Modes**: `freeze` (identical inputs), `shadow` (no side effects)

### 3.3 hitl.approve

```json
{
  "type": "hitl.approve",
  "args": {"run_id": "7f…", "node_id": "review", "decision": "approve", "reason": "LGTM"}
}
```

* **Variants**: `request_changes` with `reason`

### 3.4 graph.patch (AIR)

```json
{
  "type": "graph.patch",
  "args": {
    "run_id": "7f…",
    "patch": {
      "patch_version": "0.1",
      "add_nodes": [
        {"id":"upload_to_s3","type":"function","fn":"s3_upload","inputs":{"bucket":"acme","key_template":"uploads/${run_id}.bin"}},
        {"id":"notify_email","type":"function","fn":"email_send","inputs":{"to":"ops@acme.com","subject":"Upload OK ${run_id}"}},
        {"id":"notify_slack","type":"function","fn":"slack_post","inputs":{"channel_alias":"incidents","text":"Upload FAILED ${run_id}"}}
      ],
      "add_edges": [
        {"from":"CURRENT_NODE","to":"upload_to_s3"},
        {"from":"upload_to_s3","to":"notify_email","condition":"${nodes.upload_to_s3.outputs.success == true}"},
        {"from":"upload_to_s3","to":"notify_slack","condition":"${nodes.upload_to_s3.outputs.success == false}"}
      ],
      "justification": "Notify stakeholders after file upload"
    }
  }
}
```

* **Router → Parser**: validate against tool schemas & policy → compile overlay → Orchestrator apply

### 3.5 logs.stream

```json
{
  "type": "logs.stream",
  "args": {"run_id": "7f…", "filter": "all", "node_id": "triage"}
}
```

* **Dispatch**: Logs/Fanout → returns `{ "stream_url": "wss://…", "expires_in_sec": 90 }`

### 3.6 artifact.get

```json
{
  "type": "artifact.get",
  "args": {"artifact": "cas://sha256:…"}
}
```

* **Dispatch**: Asset/KB → presigned GET

### 3.7 cache.invalidate

```json
{
  "type": "cache.invalidate",
  "args": {"scope": "workflow", "key": "enrich_A:sha256:…"}
}
```

* **Dispatch**: Orchestrator/Cache; ensures tenant scoping

### 3.8 kb.search

```json
{
  "type": "kb.search",
  "args": {"q": "S3 403 upload", "k": 5, "filters": {"tenant": "acme"}, "hybrid": true}
}
```

* **Dispatch**: Asset/KB → hybrid (BM25 ∪ vector) → rerank

---

## 4. JSON Schemas (excerpt)

### 4.1 Envelope

```json
{
  "$id": "https://aob.dev/schemas/envelope.json",
  "type": "object",
  "required": ["version", "intent", "actor", "constraints", "sig"],
  "properties": {
    "version": {"type": "string"},
    "intent": {"$ref": "#/definitions/intent"},
    "actor": {"type": "object", "properties": {"user_id": {"type": "string"}, "tenant": {"type": "string"}, "roles": {"type": "array", "items": {"type": "string"}}}, "required": ["user_id", "tenant"]},
    "constraints": {"type": "object", "properties": {"ttl_sec": {"type": "number"}, "idempotency_key": {"type": "string"}, "capabilities": {"type": "array", "items": {"type": "string"}}}, "required": ["ttl_sec", "idempotency_key"]},
    "trace_id": {"type": "string"},
    "sig": {"type": "string"}
  },
  "definitions": {
    "intent": {"type": "object", "required": ["type", "args"], "properties": {"type": {"type": "string"}, "args": {"type": "object"}}}
  }
}
```

### 4.2 logs.stream intent

```json
{
  "$id": "https://aob.dev/schemas/intents/logs.stream.json",
  "type": "object",
  "required": ["run_id"],
  "properties": {
    "run_id": {"type": "string"},
    "filter": {"type": "string", "enum": ["all", "errors"], "default": "all"},
    "node_id": {"type": "string"}
  }
}
```

> Create similar schemas for each intent type; the Router maintains an allowlist mapping `type` → `schema`.

---

## 5. Agent Prompts (sketch)

* Use **toolformer** style prompts with explicit allowed intents and schemas.
* Temperature low (`0–0.2`), stop tokens to prevent over‑generation.
* Include examples of valid input→intent pairs.

**Example prompt excerpt**

```
You are an orchestration assistant. Produce exactly one JSON envelope per request.
Never run code or call external URLs. Use only the allowed intents and schemas.
If the user asks to stream logs, emit type=logs.stream with run_id and optional node_id.
If the user asks to patch a workflow, emit type=graph.patch with a minimal AIR patch.
```

---

## 6. Error Handling

* `SCHEMA_INVALID`: JSON fails schema; include path and reason.
* `POLICY_DENIED`: OPA/RBAC → deny; include policy id.
* `RBAC_FORBIDDEN`: actor lacks capability.
* `EXPIRED_TTL`: envelope too old.
* `CONFLICT_IDEMPOTENCY`: duplicate; return prior result.
* `MALFORMED_ARGS`: semantically wrong values (e.g., missing workflow).

---

## 7. Audit & Telemetry

* Log every accepted/rejected intent with `actor`, `intent.type`, `result`, `trace_id`.
* Metrics: intents/sec, accept rate, top denied reasons, p95 Router latency.
* Sampling: keep 100% for denied; sample accepted for cost control.

---

## 8. Sequences

See `docs/diagrams.md` for the **Agentic Session** and **Two‑call** sequences showing:

* CLI → Agentic → envelope
* CLI → Command Router → execute → Fanout stream

---

## 9. Roadmap (post‑MVP)

* Multi‑intent batches with transactional semantics.
* Attestable envelopes (TUF-style key rotation metadata).
* Human + Agent co‑authoring of complex patches (step‑by‑step).
* Intent linting and dry‑run validation mode (no side effects).
