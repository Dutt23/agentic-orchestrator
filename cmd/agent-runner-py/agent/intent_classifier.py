"""Intent classification for user prompts using LLM."""
import logging
from typing import Dict, Any, List
import json
from openai import OpenAI

logger = logging.getLogger(__name__)


class IntentClassifier:
    """Classifies user intent as 'patch' (permanent) or 'execute' (one-time) using LLM."""

    def __init__(self):
        """Initialize intent classifier with OpenAI client."""
        self.client = OpenAI()  # Uses OPENAI_API_KEY env var
        self.model = "gpt-4o-mini"  # Fast and cheap model for classification
        logger.info("Intent classifier initialized with LLM")

    def classify(self, prompt: str, context: Dict[str, Any] = None) -> Dict[str, Any]:
        """Classify user intent from prompt using LLM.

        Args:
            prompt: User's natural language prompt
            context: Optional context information

        Returns:
            Dictionary with:
                - intent: 'patch', 'execute', or 'unclear'
                - confidence: 0.0-1.0
                - reasoning: Explanation of classification
        """
        try:
            # Build context information for LLM
            context_info = ""
            if context and context.get('current_workflow'):
                workflow = context['current_workflow']
                node_count = len(workflow.get('nodes', []))
                edge_count = len(workflow.get('edges', []))
                context_info = f"\n\nCurrent workflow context: {node_count} nodes, {edge_count} edges"

            # System prompt for intent classification
            system_prompt = """You are an intent classifier for a workflow automation system. Your job is to determine the user's intent:

1. **patch** - User wants to permanently modify the workflow structure (add/remove/update nodes, edges, logic)
   - Examples: "add a node that...", "modify the workflow to...", "remove the X node", "change the condition to..."
   - Key indicators: mentions of adding/removing/modifying workflow structure

2. **execute** - User wants to execute/run a pipeline or perform a one-time data operation
   - Examples: "run this pipeline", "fetch data from...", "show me results of...", "calculate...", "process this data"
   - Key indicators: asking for results, data processing, one-time operations

3. **unclear** - Cannot determine intent with confidence

Respond ONLY with a JSON object in this exact format:
{
  "intent": "patch|execute|unclear",
  "confidence": 0.0-1.0,
  "reasoning": "brief explanation"
}"""

            user_message = f"User prompt: {prompt}{context_info}\n\nClassify the intent."

            # Call OpenAI with JSON mode
            response = self.client.chat.completions.create(
                model=self.model,
                messages=[
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": user_message}
                ],
                response_format={"type": "json_object"},
                temperature=0.3,  # Low temperature for consistent classification
                max_tokens=150
            )

            # Parse JSON response
            result_text = response.choices[0].message.content
            result = json.loads(result_text)

            # Validate and normalize result
            if 'intent' not in result or result['intent'] not in ['patch', 'execute', 'unclear']:
                logger.warning(f"Invalid intent from LLM: {result.get('intent')}, defaulting to unclear")
                result['intent'] = 'unclear'
                result['confidence'] = 0.0
                result['reasoning'] = "LLM returned invalid intent"

            if 'confidence' not in result:
                result['confidence'] = 0.5

            if 'reasoning' not in result:
                result['reasoning'] = "No reasoning provided"

            # Add metadata for backward compatibility
            result['matched_keywords'] = {'patch': [], 'execute': []}  # Empty for LLM-based classification
            result['scores'] = {
                'patch': 1.0 if result['intent'] == 'patch' else 0.0,
                'execute': 1.0 if result['intent'] == 'execute' else 0.0
            }

            logger.info(f"Intent classified by LLM: {result['intent']} (confidence: {result['confidence']:.2f}) - {result['reasoning']}")
            return result

        except Exception as e:
            logger.error(f"Failed to classify intent with LLM: {e}", exc_info=True)
            # Fallback to unclear with low confidence
            return {
                'intent': 'unclear',
                'confidence': 0.0,
                'reasoning': f"Classification failed: {str(e)}",
                'matched_keywords': {'patch': [], 'execute': []},
                'scores': {'patch': 0.0, 'execute': 0.0}
            }

    def should_constrain_tools(self, classification: Dict[str, Any], threshold: float = 0.7) -> bool:
        """Determine if we should constrain LLM to specific tool based on classification.

        Args:
            classification: Result from classify()
            threshold: Confidence threshold for constraining (0.0-1.0)

        Returns:
            True if we should constrain LLM tool choice
        """
        intent = classification['intent']
        confidence = classification['confidence']

        # Only constrain if we have high confidence and clear intent
        return intent != 'unclear' and confidence >= threshold

    def get_allowed_tools(self, classification: Dict[str, Any]) -> List[str]:
        """Get list of allowed tools based on classification.

        Args:
            classification: Result from classify()

        Returns:
            List of tool names to allow
        """
        intent = classification['intent']

        if intent == 'patch':
            return ['patch_workflow']
        elif intent == 'execute':
            return ['execute_pipeline']
        else:
            # Unclear - allow both
            return ['execute_pipeline', 'patch_workflow']
