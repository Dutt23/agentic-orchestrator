# Agent Runner Service

Python-based agentic service for LLM-powered workflow orchestration.

## Features

### 🎯 Intent Classification
Automatically determines if user request is:
- **Patch Lane**: Permanent workflow changes ("always send email when...")
- **Execute Lane**: One-time data operations ("show me top 10 flights...")

### 🔧 Two-Lane Architecture
1. **Fast Lane** (`execute_pipeline`): Ephemeral data pipelines
   - HTTP requests
   - Table operations (sort, filter, select, top_k)
   - Chainable primitives

2. **Patch Lane** (`patch_workflow`): Persistent workflow modifications
   - Add/remove/modify nodes
   - Update edges and conditions
   - JSON Patch operations

### 🧠 Context-Aware LLM
- Loads workflow schema at startup
- Receives current workflow structure
- Understands valid node types and configs
- Makes informed patching decisions

### 💾 Storage
- In-memory storage for MVP
- Stores results with metadata (tokens, execution time, etc.)
- Ready for DB/S3 integration later

## Architecture

```
Redis Job → Intent Classifier → LLM (with schema + workflow context)
                                  ↓
                        Tool Execution (execute_pipeline or patch_workflow)
                                  ↓
                        Store Result → Redis Result
```

## Project Structure

```
cmd/agent-runner-py/
├── main.py                    # Service entry point, worker pool
├── config.yaml                # Configuration
├── requirements.txt           # Dependencies
│
├── agent/
│   ├── llm_client.py         # OpenAI client with prompt caching
│   ├── intent_classifier.py  # Intent classification (patch vs execute)
│   ├── workflow_schema.py    # Schema loader and validator
│   ├── tools.py              # Tool schemas for LLM
│   └── system_prompt.py      # System prompt with schema info
│
├── pipeline/
│   ├── executor.py           # Pipeline execution engine
│   └── primitives/
│       ├── http_request.py   # HTTP GET/POST
│       └── table_ops.py      # Sort, filter, select, top_k
│
├── workflow/
│   └── patch_client.py       # Forwards patches to orchestrator API
│
├── storage/
│   ├── memory.py             # In-memory storage
│   └── redis_client.py       # Redis queue client
│
└── tests/
    ├── test_intent_classifier.py  # Intent classification tests
    ├── test_primitives.py         # Pipeline primitive tests
    └── test_integration.py        # Integration tests with mocked LLM
```

## Setup

### 1. Install Dependencies
```bash
pip install -r requirements.txt
```

### 2. Configure Environment
```bash
cp .env.example .env
# Edit .env and set:
# OPENAI_API_KEY=your_key_here
```

### 3. Configure Service
Edit `config.yaml` to set Redis, LLM, and orchestrator settings.

### 4. Run Service
```bash
python main.py
```

## Testing

```bash
# Run all tests
pytest tests/ -v

# Run specific test suite
pytest tests/test_intent_classifier.py -v

# With coverage
pytest tests/ --cov=agent --cov=pipeline --cov-report=html
```

**Note**: Tests use mocked responses and don't require OpenAI API key.

## Job Message Format

Jobs should be published to Redis queue `agent:jobs`:

```json
{
  "version": "1.0",
  "job_id": "uuid-123",
  "run_id": "run-456",
  "node_id": "agent_node",
  "workflow_tag": "main",
  "workflow_owner": "username",
  "prompt": "show me top 10 flights sorted by price",
  "current_workflow": {
    "nodes": [...],
    "edges": [...]
  },
  "context": {
    "previous_results": [...],
    "session_id": "sess-xyz"
  }
}
```

## Result Message Format

Results are published to `agent:results:{job_id}`:

```json
{
  "version": "1.0",
  "job_id": "uuid-123",
  "status": "completed",
  "result_ref": "artifact://result-uuid",
  "result_preview": {...},
  "metadata": {
    "tool_calls": [...],
    "tokens_used": 1500,
    "cache_hit": true,
    "execution_time_ms": 1200
  }
}
```

## Intent Classification Examples

### Patch Intent (Permanent)
- "always send email when price drops"
- "whenever user signs up, notify slack"
- "add notification step after processing"
- "if error occurs then alert ops team"

### Execute Intent (One-time)
- "show me top 10 flights"
- "fetch latest prices"
- "what are the most expensive items?"
- "get users who signed up today"

## HTTP Endpoints

- `GET /health` - Health check
- `GET /metrics` - Service metrics
- `POST /test/chat` - Manual testing (requires `prompt` parameter)

## Configuration

See `config.yaml` for:
- Service settings (port, workers)
- Redis connection
- LLM configuration (model, temperature, max_tokens)
- Orchestrator API URL
- Storage settings

## Development

### Adding New Primitives

1. Create function in `pipeline/primitives/`
2. Add to executor in `pipeline/executor.py`
3. Update tool schema in `agent/tools.py`
4. Add tests in `tests/test_primitives.py`

### Adding New Tools

1. Define schema in `agent/tools.py`
2. Implement execution in appropriate module
3. Add dispatcher in `main.py:_execute_tool()`
4. Update system prompt if needed

## Future Enhancements

- [ ] Database persistence (PostgreSQL)
- [ ] CAS integration for large results
- [ ] S3 storage backend
- [ ] More pipeline primitives (groupby, join, transforms)
- [ ] Additional tools (search_tools, openapi_action, delegate_to_agent)
- [ ] Multi-turn conversations with session management
- [ ] Metrics and monitoring
- [ ] Rate limiting and backoff
- [ ] Tool usage auto-promotion

## License

Internal use only.
