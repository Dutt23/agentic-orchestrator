"""Integration tests for agent service."""
import pytest
import json
from unittest.mock import Mock, patch, MagicMock
from pipeline.executor import PipelineExecutor, execute_pipeline_tool


class TestPipelineExecutor:
    """Integration tests for pipeline executor."""

    @pytest.fixture
    def executor(self):
        """Create pipeline executor instance."""
        return PipelineExecutor()

    @patch('pipeline.primitives.http_request.requests.get')
    def test_full_pipeline_execution(self, mock_get, executor):
        """Test complete pipeline execution with multiple steps."""
        # Mock HTTP response
        mock_response = Mock()
        mock_response.json.return_value = [
            {"id": 1, "name": "Item A", "price": 150},
            {"id": 2, "name": "Item B", "price": 50},
            {"id": 3, "name": "Item C", "price": 100},
            {"id": 4, "name": "Item D", "price": 200},
        ]
        mock_response.raise_for_status.return_value = None
        mock_get.return_value = mock_response

        # Pipeline: HTTP request -> Sort by price -> Top 2 -> Select name and price
        pipeline = [
            {
                "step": "http_request",
                "url": "https://api.example.com/items",
                "method": "GET"
            },
            {
                "step": "table_sort",
                "field": "price",
                "order": "asc"
            },
            {
                "step": "top_k",
                "k": 2
            },
            {
                "step": "table_select",
                "fields": ["name", "price"]
            }
        ]

        result = executor.execute(pipeline)

        assert len(result) == 2
        assert result[0] == {"name": "Item B", "price": 50}
        assert result[1] == {"name": "Item C", "price": 100}

    @patch('pipeline.primitives.http_request.requests.get')
    def test_pipeline_with_filter(self, mock_get, executor):
        """Test pipeline with filtering step."""
        mock_response = Mock()
        mock_response.json.return_value = [
            {"product": "A", "price": 25},
            {"product": "B", "price": 75},
            {"product": "C", "price": 150},
        ]
        mock_response.raise_for_status.return_value = None
        mock_get.return_value = mock_response

        pipeline = [
            {
                "step": "http_request",
                "url": "https://api.example.com/products",
                "method": "GET"
            },
            {
                "step": "table_filter",
                "condition": {"field": "price", "op": "<", "value": 100}
            }
        ]

        result = executor.execute(pipeline)

        assert len(result) == 2
        assert all(item["price"] < 100 for item in result)

    def test_pipeline_without_http(self, executor):
        """Test pipeline starting with in-memory data."""
        input_data = [
            {"id": 1, "value": 100},
            {"id": 2, "value": 50},
            {"id": 3, "value": 75},
        ]

        pipeline = [
            {"step": "table_sort", "field": "value", "order": "desc"},
            {"step": "top_k", "k": 2}
        ]

        result = executor.execute(pipeline, input_data=input_data)

        assert len(result) == 2
        assert result[0]["value"] == 100
        assert result[1]["value"] == 75

    def test_execute_pipeline_tool(self):
        """Test execute_pipeline_tool wrapper."""
        args = {
            "session_id": "test-session",
            "pipeline": [
                {"step": "top_k", "k": 1}
            ]
        }

        # Note: This will fail without input_data, but tests the structure
        with pytest.raises(ValueError):
            execute_pipeline_tool(args)


class TestAgentServiceIntegration:
    """Integration tests for agent service with mocked LLM."""

    @pytest.fixture
    def mock_llm_response(self):
        """Create mock LLM response for execute_pipeline."""
        return {
            "tool_calls": [
                {
                    "id": "call_123",
                    "function": {
                        "name": "execute_pipeline",
                        "arguments": json.dumps({
                            "session_id": "test",
                            "pipeline": [
                                {"step": "table_sort", "field": "price", "order": "asc"},
                                {"step": "top_k", "k": 3}
                            ]
                        })
                    }
                }
            ],
            "tokens_used": 500,
            "cache_hit": False,
            "execution_time_ms": 1200,
            "model": "gpt-4o",
            "finish_reason": "stop"
        }

    @pytest.fixture
    def mock_llm_patch_response(self):
        """Create mock LLM response for patch_workflow."""
        return {
            "tool_calls": [
                {
                    "id": "call_456",
                    "function": {
                        "name": "patch_workflow",
                        "arguments": json.dumps({
                            "workflow_tag": "main",
                            "workflow_owner": "test_user",
                            "patch_spec": {
                                "operations": [
                                    {
                                        "op": "add",
                                        "path": "/nodes/-",
                                        "value": {
                                            "id": "email_notifier",
                                            "type": "function",
                                            "config": {"handler": "send_email"}
                                        }
                                    }
                                ],
                                "description": "Add email notification"
                            }
                        })
                    }
                }
            ],
            "tokens_used": 600,
            "cache_hit": False,
            "execution_time_ms": 1500,
            "model": "gpt-4o",
            "finish_reason": "stop"
        }

    def test_intent_classification_for_execute(self):
        """Test intent classification correctly identifies execute intent."""
        from agent.intent_classifier import IntentClassifier

        classifier = IntentClassifier()
        result = classifier.classify("show me top 10 items sorted by price")

        assert result['intent'] == 'execute'
        assert result['confidence'] > 0.4

    def test_intent_classification_for_patch(self):
        """Test intent classification correctly identifies patch intent."""
        from agent.intent_classifier import IntentClassifier

        classifier = IntentClassifier()
        result = classifier.classify("always send email when price drops below 500")

        assert result['intent'] == 'patch'
        assert result['confidence'] > 0.6

    @patch('agent.llm_client.OpenAI')
    def test_llm_client_with_workflow_context(self, mock_openai_class):
        """Test LLM client includes workflow context in messages."""
        from agent.llm_client import LLMClient

        # Mock OpenAI client
        mock_client = MagicMock()
        mock_response = MagicMock()
        mock_response.choices = [MagicMock()]
        mock_response.choices[0].message.tool_calls = []
        mock_response.choices[0].finish_reason = "stop"
        mock_response.usage.total_tokens = 100
        mock_client.chat.completions.create.return_value = mock_response
        mock_openai_class.return_value = mock_client

        config = {
            "model": "gpt-4o",
            "temperature": 0.1,
            "max_tokens": 4000,
            "timeout_sec": 30
        }

        llm = LLMClient(config)

        # Call with workflow context
        context = {
            "current_workflow": {
                "nodes": [
                    {"id": "node1", "type": "http", "config": {"url": "test"}},
                    {"id": "node2", "type": "function"}
                ],
                "edges": [
                    {"from": "node1", "to": "node2"}
                ]
            },
            "session_id": "test-session"
        }

        llm.chat("add email notification", context)

        # Verify the call was made
        assert mock_client.chat.completions.create.called

        # Check the messages argument
        call_args = mock_client.chat.completions.create.call_args
        messages = call_args[1]['messages']

        # Should have system and user messages
        assert len(messages) == 2
        assert messages[0]['role'] == 'system'
        assert messages[1]['role'] == 'user'

        # User message should contain workflow structure
        user_content = messages[1]['content']
        assert 'Current Workflow Structure' in user_content
        assert 'node1' in user_content
        assert 'node2' in user_content

    def test_storage_memory(self):
        """Test in-memory storage."""
        from storage.memory import MemoryStorage

        storage = MemoryStorage()

        result_id = storage.store_result(
            job_id="job-123",
            run_id="run-456",
            node_id="node-789",
            result_data={"test": "data"},
            status="completed",
            tokens_used=500
        )

        assert result_id is not None

        # Retrieve result
        result = storage.get_result(result_id)
        assert result is not None
        assert result['job_id'] == "job-123"
        assert result['result_data'] == {"test": "data"}

        # Retrieve by job_id
        result_by_job = storage.get_result_by_job_id("job-123")
        assert result_by_job is not None
        assert result_by_job['result_id'] == result_id


class TestWorkflowSchema:
    """Test workflow schema loader."""

    def test_schema_loading(self):
        """Test workflow schema loading."""
        from agent.workflow_schema import WorkflowSchema

        schema = WorkflowSchema()

        assert schema.node_types is not None
        assert len(schema.node_types) > 0
        assert 'function' in schema.node_types
        assert 'http' in schema.node_types

    def test_schema_summary(self):
        """Test schema summary generation."""
        from agent.workflow_schema import WorkflowSchema

        schema = WorkflowSchema()
        summary = schema.get_schema_summary()

        assert 'Valid Node Types' in summary
        assert 'function' in summary
        assert 'http' in summary
        assert 'Node Structure' in summary

    def test_node_type_validation(self):
        """Test node type validation."""
        from agent.workflow_schema import WorkflowSchema

        schema = WorkflowSchema()

        assert schema.validate_node_type('function') == True
        assert schema.validate_node_type('http') == True
        assert schema.validate_node_type('invalid_type') == False


if __name__ == '__main__':
    pytest.main([__file__, '-v'])
