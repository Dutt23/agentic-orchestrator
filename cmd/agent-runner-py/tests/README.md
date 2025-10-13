# Agent Runner Tests

## Running Tests

### Install dependencies
```bash
pip install -r requirements.txt
```

### Run all tests (excluding real LLM tests)
```bash
pytest tests/ -v
```

### Run specific test file
```bash
pytest tests/test_intent_classifier.py -v
pytest tests/test_primitives.py -v
pytest tests/test_integration.py -v
```

### Run with coverage
```bash
pytest tests/ --cov=agent --cov=pipeline --cov=storage --cov-report=html
```

### Run REAL LLM tests (requires OPENAI_API_KEY)
```bash
# Set your API key first
export OPENAI_API_KEY=sk-...

# Run real LLM integration tests (costs ~5-10 cents)
pytest tests/test_real_llm.py -m llm -v -s

# Or run specific test
pytest tests/test_real_llm.py::TestRealLLMIntegration::test_execute_intent_simple_query -m llm -v -s
```

### Skip LLM tests explicitly
```bash
pytest tests/ -m "not llm" -v
```

## Test Structure

- `test_intent_classifier.py` - Tests for intent classification (patch vs execute)
- `test_primitives.py` - Tests for pipeline primitives (http_request, table_ops)
- `test_integration.py` - Integration tests with mocked LLM
- `test_real_llm.py` - **Real OpenAI API tests** (requires API key, costs money)

## Note on OpenAI Key

**Mocked Tests** (default):
- `test_intent_classifier.py` - No API key needed ✓
- `test_primitives.py` - No API key needed ✓
- `test_integration.py` - No API key needed ✓

**Real LLM Tests** (optional, marked with `@pytest.mark.llm`):
- `test_real_llm.py` - **Requires OPENAI_API_KEY** and costs ~5-10 cents
- Skipped by default unless you run with `-m llm`
- Uses `gpt-4o-mini` model to minimize cost
- Tests actual LLM tool selection and intent agreement

## Expected Test Results

**Mocked tests** (run by default):
- ✓ Intent classification tests (keyword-based, no LLM needed)
- ✓ Pipeline primitive tests (mocked HTTP calls)
- ✓ Integration tests (mocked LLM responses)
- Total: ~40 tests, all pass without API key

**Real LLM tests** (run with `-m llm`):
- ✓ Test 1: Execute intent - simple query
- ✓ Test 2: Patch intent - always statement
- ✓ Test 3: Execute with workflow context
- ✓ Test 4: Patch with workflow context
- ✓ Test 5: Intent classifier vs LLM agreement (tests 4 prompts)
- Total: 5 tests, requires API key, costs ~5-10 cents
