# Quick Test Reference

## 🚀 Running Tests

### 1. Install Dependencies (first time only)
```bash
cd /Users/sdutt/Documents/practice/lyzr/orchestrator/cmd/agent-runner-py
pip3 install -r requirements.txt
```

### 2. Run Mocked Tests (FREE - no API key needed)
```bash
# All mocked tests
pytest tests/ -v

# Or specific files
pytest tests/test_intent_classifier.py -v
pytest tests/test_primitives.py -v
pytest tests/test_integration.py -v
```

### 3. Run REAL OpenAI Tests (costs ~5-10 cents)
```bash
# Make sure OPENAI_API_KEY is set in .env
# Then run:
pytest tests/test_real_llm.py -m llm -v -s
```

---

## 📊 What Each Test Does

### Mocked Tests (40+ tests, FREE)

#### `test_intent_classifier.py` (20 tests)
- ✓ Detects "patch" intent: `"always send email"` → patch
- ✓ Detects "execute" intent: `"show me data"` → execute
- ✓ Handles edge cases and ambiguous prompts

#### `test_primitives.py` (15 tests)
- ✓ Table operations: sort, filter, select, top_k
- ✓ HTTP requests: GET, POST with params
- ✓ Chained pipeline execution

#### `test_integration.py` (10 tests)
- ✓ Full pipeline execution
- ✓ Workflow schema loading
- ✓ Storage operations
- ✓ LLM client setup (mocked)

---

### Real LLM Tests (5 tests, ~$0.05-0.10)

#### `test_real_llm.py`
1. **test_execute_intent_simple_query**
   - Prompt: "show me top 5 items sorted by price"
   - Verifies: LLM returns `execute_pipeline` tool
   - Checks: Pipeline has sort + top_k steps

2. **test_patch_intent_always_statement**
   - Prompt: "always send email notification when price < 500"
   - Verifies: LLM returns `patch_workflow` tool
   - Checks: Patch adds nodes/edges

3. **test_execute_with_workflow_context**
   - Gives: Existing workflow with 3 nodes
   - Prompt: "fetch items and filter by category A"
   - Verifies: LLM understands context and creates pipeline

4. **test_patch_with_workflow_context**
   - Gives: Existing workflow
   - Prompt: "add email notification after process_data node"
   - Verifies: LLM references existing node IDs correctly

5. **test_intent_classifier_llm_agreement**
   - Tests 4 different prompts
   - Compares: Intent classifier vs LLM tool choice
   - Checks: Agreement rate ≥75%

---

## 🎯 Quick Commands

```bash
# Regular workflow
pytest tests/ -v                                    # Mocked tests only (default)

# With real LLM
pytest tests/test_real_llm.py -m llm -v -s         # All 5 real LLM tests

# Run just one real LLM test
pytest tests/test_real_llm.py::TestRealLLMIntegration::test_execute_intent_simple_query -m llm -v -s

# Skip LLM tests explicitly
pytest tests/ -m "not llm" -v

# With coverage report
pytest tests/ --cov=agent --cov=pipeline --cov-report=html
```

---

## 💰 Cost Estimate

**Mocked tests**: FREE (no API calls)
**Real LLM tests**: ~$0.05-0.10 total
- Uses `gpt-4o-mini` (cheaper model)
- ~5-6 API calls total
- ~500-1000 tokens per call

---

## ✅ Expected Results

All tests should pass! If any fail, check:
1. OPENAI_API_KEY is set (for real LLM tests)
2. Dependencies installed: `pip3 install -r requirements.txt`
3. Python version ≥3.8
