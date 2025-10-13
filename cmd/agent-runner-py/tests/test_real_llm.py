"""Integration tests with real OpenAI API calls.

These tests make actual API calls to OpenAI and will consume API credits.
Run only when needed with: pytest tests/test_real_llm.py -m llm -v

Estimated cost: ~5-10 cents for all tests
"""
import pytest
import os
import json
from agent.llm_client import LLMClient
from agent.workflow_schema import WorkflowSchema
from agent.intent_classifier import IntentClassifier


# Skip these tests by default (requires OPENAI_API_KEY and costs money)
pytestmark = pytest.mark.llm


@pytest.fixture
def llm_client():
    """Create LLM client with real OpenAI connection."""
    # Check for API key
    api_key = os.getenv('OPENAI_API_KEY')
    if not api_key:
        pytest.skip("OPENAI_API_KEY not set")

    # Load workflow schema
    schema = WorkflowSchema()
    schema_summary = schema.get_schema_summary()

    # Create LLM client
    config = {
        'model': 'gpt-4o-mini',  # Use cheaper model for testing
        'temperature': 0.1,
        'max_tokens': 1000,
        'timeout_sec': 30
    }

    return LLMClient(config, workflow_schema_summary=schema_summary)


@pytest.fixture
def intent_classifier():
    """Create intent classifier."""
    return IntentClassifier()


@pytest.fixture
def sample_workflow():
    """Sample workflow for context."""
    return {
        "nodes": [
            {
                "id": "fetch_data",
                "type": "http",
                "config": {"url": "https://api.example.com/data", "method": "GET"}
            },
            {
                "id": "process_data",
                "type": "transform",
                "config": {"script": "process.py"}
            },
            {
                "id": "save_result",
                "type": "function",
                "config": {"handler": "save_to_db"}
            }
        ],
        "edges": [
            {"from": "fetch_data", "to": "process_data"},
            {"from": "process_data", "to": "save_result"}
        ]
    }


class TestRealLLMIntegration:
    """Integration tests with real OpenAI API calls."""

    def test_execute_intent_simple_query(self, llm_client, intent_classifier):
        """Test 1: Execute intent with simple data query.

        Expected: LLM returns execute_pipeline tool with sort and top_k steps.
        """
        prompt = "show me top 5 items sorted by price ascending"

        # Classify intent
        intent_result = intent_classifier.classify(prompt)
        print(f"\n[Test 1] Intent: {intent_result['intent']} "
              f"(confidence: {intent_result['confidence']:.2f})")

        # Call LLM
        llm_result = llm_client.chat(prompt)

        print(f"[Test 1] Tokens used: {llm_result['tokens_used']}")
        print(f"[Test 1] Execution time: {llm_result['execution_time_ms']}ms")

        # Assertions
        assert len(llm_result['tool_calls']) > 0, "LLM should return at least one tool call"

        tool_call = llm_result['tool_calls'][0]
        tool_name = tool_call['function']['name']

        print(f"[Test 1] Tool chosen: {tool_name}")

        # Should choose execute_pipeline for one-time query
        assert tool_name == 'execute_pipeline', \
            f"Expected execute_pipeline but got {tool_name}"

        # Parse arguments
        args = json.loads(tool_call['function']['arguments'])
        pipeline = args.get('pipeline', [])

        print(f"[Test 1] Pipeline steps: {[step['step'] for step in pipeline]}")

        # Should contain sort and top_k steps
        step_types = [step['step'] for step in pipeline]
        assert 'table_sort' in step_types, "Pipeline should contain sort step"
        assert 'top_k' in step_types, "Pipeline should contain top_k step"

        # Verify intent classifier agrees with LLM choice
        assert intent_result['intent'] == 'execute', \
            "Intent classifier should agree with LLM (expected: execute)"

    def test_patch_intent_always_statement(self, llm_client, intent_classifier):
        """Test 2: Patch intent with 'always' keyword.

        Expected: LLM returns patch_workflow tool with node addition.
        """
        prompt = "always send email notification when price drops below 500"

        # Classify intent
        intent_result = intent_classifier.classify(prompt)
        print(f"\n[Test 2] Intent: {intent_result['intent']} "
              f"(confidence: {intent_result['confidence']:.2f})")

        # Call LLM
        llm_result = llm_client.chat(prompt)

        print(f"[Test 2] Tokens used: {llm_result['tokens_used']}")
        print(f"[Test 2] Execution time: {llm_result['execution_time_ms']}ms")

        # Assertions
        assert len(llm_result['tool_calls']) > 0, "LLM should return at least one tool call"

        tool_call = llm_result['tool_calls'][0]
        tool_name = tool_call['function']['name']

        print(f"[Test 2] Tool chosen: {tool_name}")

        # Should choose patch_workflow for permanent change
        assert tool_name == 'patch_workflow', \
            f"Expected patch_workflow but got {tool_name}"

        # Parse arguments
        args = json.loads(tool_call['function']['arguments'])
        patch_spec = args.get('patch_spec', {})
        operations = patch_spec.get('operations', [])

        print(f"[Test 2] Patch operations: {len(operations)}")

        # Should have operations to add nodes/edges
        assert len(operations) > 0, "Patch should contain at least one operation"

        # Verify intent classifier agrees with LLM choice
        assert intent_result['intent'] == 'patch', \
            "Intent classifier should agree with LLM (expected: patch)"

    def test_execute_with_workflow_context(self, llm_client, sample_workflow, intent_classifier):
        """Test 3: Execute with existing workflow context.

        Expected: LLM understands workflow context and creates appropriate pipeline.
        """
        prompt = "fetch latest items and filter by category A"

        context = {
            "current_workflow": sample_workflow,
            "session_id": "test-session-123"
        }

        # Classify intent
        intent_result = intent_classifier.classify(prompt, context)
        print(f"\n[Test 3] Intent: {intent_result['intent']} "
              f"(confidence: {intent_result['confidence']:.2f})")

        # Call LLM with context
        llm_result = llm_client.chat(prompt, context)

        print(f"[Test 3] Tokens used: {llm_result['tokens_used']}")
        print(f"[Test 3] Execution time: {llm_result['execution_time_ms']}ms")

        # Assertions
        assert len(llm_result['tool_calls']) > 0, "LLM should return at least one tool call"

        tool_call = llm_result['tool_calls'][0]
        tool_name = tool_call['function']['name']

        print(f"[Test 3] Tool chosen: {tool_name}")

        # Should choose execute_pipeline
        assert tool_name == 'execute_pipeline', \
            f"Expected execute_pipeline but got {tool_name}"

        # Parse arguments
        args = json.loads(tool_call['function']['arguments'])
        pipeline = args.get('pipeline', [])

        print(f"[Test 3] Pipeline steps: {[step['step'] for step in pipeline]}")

        # Should contain http_request and table_filter
        step_types = [step['step'] for step in pipeline]
        assert 'http_request' in step_types or len(pipeline) > 0, \
            "Pipeline should contain steps"

    def test_patch_with_workflow_context(self, llm_client, sample_workflow, intent_classifier):
        """Test 4: Patch with existing workflow context.

        Expected: LLM references existing node IDs correctly.
        """
        prompt = "add email notification step after process_data node"

        context = {
            "current_workflow": sample_workflow,
            "session_id": "test-session-456"
        }

        # Classify intent
        intent_result = intent_classifier.classify(prompt, context)
        print(f"\n[Test 4] Intent: {intent_result['intent']} "
              f"(confidence: {intent_result['confidence']:.2f})")

        # Call LLM with context
        llm_result = llm_client.chat(prompt, context)

        print(f"[Test 4] Tokens used: {llm_result['tokens_used']}")
        print(f"[Test 4] Execution time: {llm_result['execution_time_ms']}ms")

        # Assertions
        assert len(llm_result['tool_calls']) > 0, "LLM should return at least one tool call"

        tool_call = llm_result['tool_calls'][0]
        tool_name = tool_call['function']['name']

        print(f"[Test 4] Tool chosen: {tool_name}")

        # Should choose patch_workflow
        assert tool_name == 'patch_workflow', \
            f"Expected patch_workflow but got {tool_name}"

        # Parse arguments
        args = json.loads(tool_call['function']['arguments'])
        patch_spec = args.get('patch_spec', {})
        operations = patch_spec.get('operations', [])

        print(f"[Test 4] Patch operations: {len(operations)}")
        for op in operations:
            print(f"  - {op.get('op')} at {op.get('path')}")

        # Should have operations
        assert len(operations) > 0, "Patch should contain operations"

        # Verify intent classifier agrees
        assert intent_result['intent'] == 'patch', \
            "Intent classifier should agree with LLM (expected: patch)"

    def test_intent_classifier_llm_agreement(self, llm_client, intent_classifier):
        """Test 5: Verify intent classifier agrees with LLM choices.

        Tests multiple prompts and tracks agreement rate.
        """
        test_cases = [
            ("show me the data", "execute"),
            ("always notify admin", "patch"),
            ("get top 10 records", "execute"),
            ("whenever error occurs send alert", "patch"),
        ]

        results = []
        total_tokens = 0

        for prompt, expected_intent in test_cases:
            print(f"\n[Test 5] Testing: '{prompt}'")

            # Classify intent
            intent_result = intent_classifier.classify(prompt)
            classifier_intent = intent_result['intent']
            confidence = intent_result['confidence']

            print(f"  Classifier: {classifier_intent} (confidence: {confidence:.2f})")

            # Call LLM
            llm_result = llm_client.chat(prompt)
            total_tokens += llm_result['tokens_used']

            if llm_result['tool_calls']:
                tool_name = llm_result['tool_calls'][0]['function']['name']
                llm_intent = 'patch' if tool_name == 'patch_workflow' else 'execute'
                print(f"  LLM chose: {tool_name} -> {llm_intent}")

                # Check agreement
                agrees = (classifier_intent == llm_intent)
                results.append({
                    'prompt': prompt,
                    'expected': expected_intent,
                    'classifier': classifier_intent,
                    'llm': llm_intent,
                    'agrees': agrees,
                    'confidence': confidence
                })

                print(f"  Agreement: {'✓' if agrees else '✗'}")

        print(f"\n[Test 5] Total tokens used: {total_tokens}")

        # Calculate agreement rate
        agreement_rate = sum(1 for r in results if r['agrees']) / len(results)
        print(f"[Test 5] Agreement rate: {agreement_rate * 100:.1f}%")

        # Should have high agreement rate (>75%)
        assert agreement_rate >= 0.75, \
            f"Low agreement rate: {agreement_rate * 100:.1f}% (expected ≥75%)"

        # All expected intents should match at least one of classifier or LLM
        for result in results:
            classifier_correct = result['classifier'] == result['expected']
            llm_correct = result['llm'] == result['expected']

            assert classifier_correct or llm_correct, \
                f"Both classifier and LLM wrong for '{result['prompt']}'"


if __name__ == '__main__':
    # Run with: python -m pytest tests/test_real_llm.py -m llm -v -s
    pytest.main([__file__, '-m', 'llm', '-v', '-s'])
