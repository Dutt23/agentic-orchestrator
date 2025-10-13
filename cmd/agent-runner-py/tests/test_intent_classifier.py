"""Tests for intent classifier."""
import pytest
from agent.intent_classifier import IntentClassifier


class TestIntentClassifier:
    """Test suite for IntentClassifier."""

    @pytest.fixture
    def classifier(self):
        """Create intent classifier instance."""
        return IntentClassifier()

    def test_patch_intent_with_always(self, classifier):
        """Test patch intent detection with 'always' keyword."""
        result = classifier.classify("always send email when price drops below 500")

        assert result['intent'] == 'patch'
        assert result['confidence'] > 0.5
        assert 'always' in result['matched_keywords']['patch']

    def test_patch_intent_with_whenever(self, classifier):
        """Test patch intent detection with 'whenever' keyword."""
        result = classifier.classify("whenever a new order arrives, send notification to slack")

        assert result['intent'] == 'patch'
        assert result['confidence'] > 0.5
        assert 'whenever' in result['matched_keywords']['patch']

    def test_patch_intent_with_conditional(self, classifier):
        """Test patch intent detection with conditional pattern."""
        result = classifier.classify("if price < 500 then send email to ops team")

        assert result['intent'] == 'patch'
        assert result['confidence'] > 0.5

    def test_patch_intent_add_node(self, classifier):
        """Test patch intent when adding workflow nodes."""
        result = classifier.classify("add email notification node after processing")

        assert result['intent'] == 'patch'
        assert result['confidence'] > 0.5

    def test_execute_intent_with_show(self, classifier):
        """Test execute intent detection with 'show' keyword."""
        result = classifier.classify("show me all flights from NYC to LAX")

        assert result['intent'] == 'execute'
        assert result['confidence'] > 0.4
        assert 'show' in result['matched_keywords']['execute']

    def test_execute_intent_with_fetch(self, classifier):
        """Test execute intent detection with 'fetch' keyword."""
        result = classifier.classify("fetch latest prices and sort by cheapest")

        assert result['intent'] == 'execute'
        assert result['confidence'] > 0.4
        assert 'fetch' in result['matched_keywords']['execute']

    def test_execute_intent_with_question(self, classifier):
        """Test execute intent detection with question mark."""
        result = classifier.classify("what are the top 10 most expensive items?")

        assert result['intent'] == 'execute'
        assert result['confidence'] > 0.4
        assert 'question' in result['matched_keywords']['execute']

    def test_execute_intent_get_data(self, classifier):
        """Test execute intent for getting data."""
        result = classifier.classify("get me the list of users who signed up today")

        assert result['intent'] == 'execute'
        assert result['confidence'] > 0.4

    def test_unclear_intent_ambiguous(self, classifier):
        """Test unclear intent for ambiguous prompts."""
        result = classifier.classify("process the data")

        # Should be unclear or have low confidence
        assert result['intent'] == 'unclear' or result['confidence'] < 0.5

    def test_patch_vs_execute_scoring(self, classifier):
        """Test that patch keywords score higher than execute for permanent changes."""
        patch_result = classifier.classify("always notify admin when error occurs")
        execute_result = classifier.classify("show me current errors")

        assert patch_result['scores']['patch'] > execute_result['scores']['patch']
        assert execute_result['scores']['execute'] > patch_result['scores']['execute']

    def test_temporal_with_action(self, classifier):
        """Test temporal indicator combined with action verb."""
        result = classifier.classify("when user signs up send welcome email")

        assert result['intent'] == 'patch'
        assert result['confidence'] > 0.6

    def test_modification_verbs(self, classifier):
        """Test workflow modification verbs."""
        prompts = [
            "add notification step after processing",
            "remove the logging node",
            "update the email configuration",
            "modify the conditional logic"
        ]

        for prompt in prompts:
            result = classifier.classify(prompt)
            assert result['intent'] == 'patch', f"Failed for: {prompt}"

    def test_should_constrain_tools_high_confidence(self, classifier):
        """Test tool constraining with high confidence."""
        result = classifier.classify("always send email when price drops")

        should_constrain = classifier.should_constrain_tools(result, threshold=0.7)
        assert should_constrain == True

    def test_should_not_constrain_low_confidence(self, classifier):
        """Test no constraining with low confidence."""
        result = classifier.classify("do something")

        should_constrain = classifier.should_constrain_tools(result, threshold=0.7)
        assert should_constrain == False

    def test_get_allowed_tools_patch(self, classifier):
        """Test allowed tools for patch intent."""
        result = classifier.classify("always send notification")

        tools = classifier.get_allowed_tools(result)
        assert tools == ['patch_workflow']

    def test_get_allowed_tools_execute(self, classifier):
        """Test allowed tools for execute intent."""
        result = classifier.classify("show me the data")

        tools = classifier.get_allowed_tools(result)
        assert tools == ['execute_pipeline']

    def test_get_allowed_tools_unclear(self, classifier):
        """Test allowed tools for unclear intent."""
        result = classifier.classify("something vague")

        tools = classifier.get_allowed_tools(result)
        assert set(tools) == {'execute_pipeline', 'patch_workflow'}

    def test_complex_patch_scenario(self, classifier):
        """Test complex patch scenario."""
        prompt = """
        Whenever the price drops below $500, automatically send an email
        to the operations team and create a notification in Slack. This
        should happen every time the condition is met.
        """

        result = classifier.classify(prompt)
        assert result['intent'] == 'patch'
        assert result['confidence'] > 0.7
        assert len(result['matched_keywords']['patch']) > 2

    def test_complex_execute_scenario(self, classifier):
        """Test complex execute scenario."""
        prompt = """
        Show me the top 10 flights from NYC to LAX, sorted by price,
        and filter out any flights that depart before 8am.
        """

        result = classifier.classify(prompt)
        assert result['intent'] == 'execute'
        assert result['confidence'] > 0.5


if __name__ == '__main__':
    pytest.main([__file__, '-v'])
