"""Tests for pipeline primitives."""
import pytest
from unittest.mock import Mock, patch
from pipeline.primitives import table_ops
from pipeline.primitives import http_request


class TestTableOperations:
    """Test suite for table operation primitives."""

    @pytest.fixture
    def sample_data(self):
        """Sample data for testing."""
        return [
            {"id": 1, "name": "Alice", "price": 100, "category": "A"},
            {"id": 2, "name": "Bob", "price": 50, "category": "B"},
            {"id": 3, "name": "Charlie", "price": 150, "category": "A"},
            {"id": 4, "name": "David", "price": 75, "category": "B"},
            {"id": 5, "name": "Eve", "price": 200, "category": "A"},
        ]

    def test_table_sort_asc(self, sample_data):
        """Test sorting in ascending order."""
        step = {"step": "table_sort", "field": "price", "order": "asc"}
        result = table_ops.table_sort(step, sample_data)

        assert len(result) == 5
        assert result[0]["price"] == 50
        assert result[-1]["price"] == 200

    def test_table_sort_desc(self, sample_data):
        """Test sorting in descending order."""
        step = {"step": "table_sort", "field": "price", "order": "desc"}
        result = table_ops.table_sort(step, sample_data)

        assert len(result) == 5
        assert result[0]["price"] == 200
        assert result[-1]["price"] == 50

    def test_table_sort_by_name(self, sample_data):
        """Test sorting by string field."""
        step = {"step": "table_sort", "field": "name", "order": "asc"}
        result = table_ops.table_sort(step, sample_data)

        assert result[0]["name"] == "Alice"
        assert result[-1]["name"] == "Eve"

    def test_table_sort_missing_field(self, sample_data):
        """Test sorting with missing field."""
        step = {"step": "table_sort", "order": "asc"}

        with pytest.raises(ValueError, match="requires 'field' parameter"):
            table_ops.table_sort(step, sample_data)

    def test_table_filter_less_than(self, sample_data):
        """Test filtering with less than operator."""
        step = {
            "step": "table_filter",
            "condition": {"field": "price", "op": "<", "value": 100}
        }
        result = table_ops.table_filter(step, sample_data)

        assert len(result) == 2
        assert all(item["price"] < 100 for item in result)

    def test_table_filter_greater_than(self, sample_data):
        """Test filtering with greater than operator."""
        step = {
            "step": "table_filter",
            "condition": {"field": "price", "op": ">", "value": 100}
        }
        result = table_ops.table_filter(step, sample_data)

        assert len(result) == 2
        assert all(item["price"] > 100 for item in result)

    def test_table_filter_equals(self, sample_data):
        """Test filtering with equals operator."""
        step = {
            "step": "table_filter",
            "condition": {"field": "category", "op": "==", "value": "A"}
        }
        result = table_ops.table_filter(step, sample_data)

        assert len(result) == 3
        assert all(item["category"] == "A" for item in result)

    def test_table_filter_not_equals(self, sample_data):
        """Test filtering with not equals operator."""
        step = {
            "step": "table_filter",
            "condition": {"field": "category", "op": "!=", "value": "A"}
        }
        result = table_ops.table_filter(step, sample_data)

        assert len(result) == 2
        assert all(item["category"] != "A" for item in result)

    def test_table_select(self, sample_data):
        """Test selecting specific fields."""
        step = {"step": "table_select", "fields": ["id", "name"]}
        result = table_ops.table_select(step, sample_data)

        assert len(result) == 5
        for item in result:
            assert set(item.keys()) == {"id", "name"}
            assert "price" not in item
            assert "category" not in item

    def test_table_select_single_field(self, sample_data):
        """Test selecting single field."""
        step = {"step": "table_select", "fields": ["name"]}
        result = table_ops.table_select(step, sample_data)

        assert len(result) == 5
        for item in result:
            assert list(item.keys()) == ["name"]

    def test_top_k(self, sample_data):
        """Test taking top k records."""
        step = {"step": "top_k", "k": 3}
        result = table_ops.top_k(step, sample_data)

        assert len(result) == 3
        assert result[0]["id"] == 1
        assert result[1]["id"] == 2
        assert result[2]["id"] == 3

    def test_top_k_more_than_available(self, sample_data):
        """Test top k when k > data length."""
        step = {"step": "top_k", "k": 10}
        result = table_ops.top_k(step, sample_data)

        assert len(result) == 5  # Should return all available

    def test_top_k_one(self, sample_data):
        """Test taking top 1 record."""
        step = {"step": "top_k", "k": 1}
        result = table_ops.top_k(step, sample_data)

        assert len(result) == 1
        assert result[0]["id"] == 1

    def test_execute_dispatcher(self, sample_data):
        """Test execute function dispatcher."""
        step = {"step": "table_sort", "field": "price", "order": "asc"}
        result = table_ops.execute(step, sample_data)

        assert len(result) == 5
        assert result[0]["price"] == 50

    def test_chained_operations(self, sample_data):
        """Test chaining multiple table operations."""
        # Filter -> Sort -> Top K -> Select
        filtered = table_ops.table_filter(
            {"step": "table_filter", "condition": {"field": "price", "op": ">", "value": 50}},
            sample_data
        )
        sorted_data = table_ops.table_sort(
            {"step": "table_sort", "field": "price", "order": "asc"},
            filtered
        )
        top = table_ops.top_k(
            {"step": "top_k", "k": 2},
            sorted_data
        )
        selected = table_ops.table_select(
            {"step": "table_select", "fields": ["name", "price"]},
            top
        )

        assert len(selected) == 2
        assert set(selected[0].keys()) == {"name", "price"}
        assert selected[0]["price"] == 75  # David


class TestHttpRequest:
    """Test suite for HTTP request primitive."""

    @patch('pipeline.primitives.http_request.requests.get')
    def test_http_get_success(self, mock_get):
        """Test successful HTTP GET request."""
        mock_response = Mock()
        mock_response.json.return_value = {"data": "test"}
        mock_response.raise_for_status.return_value = None
        mock_get.return_value = mock_response

        step = {
            "step": "http_request",
            "url": "https://api.example.com/data",
            "method": "GET"
        }

        result = http_request.execute(step, None)

        assert result == {"data": "test"}
        mock_get.assert_called_once()

    @patch('pipeline.primitives.http_request.requests.post')
    def test_http_post_success(self, mock_post):
        """Test successful HTTP POST request."""
        mock_response = Mock()
        mock_response.json.return_value = {"success": True}
        mock_response.raise_for_status.return_value = None
        mock_post.return_value = mock_response

        step = {
            "step": "http_request",
            "url": "https://api.example.com/data",
            "method": "POST",
            "params": {"key": "value"}
        }

        result = http_request.execute(step, None)

        assert result == {"success": True}
        mock_post.assert_called_once_with(
            "https://api.example.com/data",
            json={"key": "value"},
            timeout=30
        )

    def test_http_missing_url(self):
        """Test HTTP request without URL."""
        step = {"step": "http_request", "method": "GET"}

        with pytest.raises(ValueError, match="requires 'url' parameter"):
            http_request.execute(step, None)

    @patch('pipeline.primitives.http_request.requests.get')
    def test_http_get_with_params(self, mock_get):
        """Test HTTP GET with query parameters."""
        mock_response = Mock()
        mock_response.json.return_value = []
        mock_response.raise_for_status.return_value = None
        mock_get.return_value = mock_response

        step = {
            "step": "http_request",
            "url": "https://api.example.com/search",
            "method": "GET",
            "params": {"q": "test", "limit": 10}
        }

        http_request.execute(step, None)

        mock_get.assert_called_once_with(
            "https://api.example.com/search",
            params={"q": "test", "limit": 10},
            timeout=30
        )


if __name__ == '__main__':
    pytest.main([__file__, '-v'])
