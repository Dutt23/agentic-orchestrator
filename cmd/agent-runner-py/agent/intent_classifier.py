"""Intent classification for user prompts."""
import logging
from typing import Dict, Any, List
import re

logger = logging.getLogger(__name__)


class IntentClassifier:
    """Classifies user intent as 'patch' (permanent) or 'execute' (one-time)."""

    # Keywords indicating permanent changes (patch lane)
    PATCH_KEYWORDS = [
        'always', 'whenever', 'every time', 'from now on',
        'permanently', 'forever', 'each time', 'all the time',
        'every', 'schedule', 'recurring', 'automated',
        'automatically', 'continuous', 'ongoing'
    ]

    # Keywords indicating one-time operations (execute lane)
    EXECUTE_KEYWORDS = [
        'show', 'fetch', 'get', 'display', 'find',
        'list', 'retrieve', 'search', 'look up',
        'give me', 'tell me', 'what is', 'what are',
        'how many', 'check', 'see', 'view'
    ]

    # Temporal indicators for patch
    TEMPORAL_PATCH = [
        'when', 'if', 'while', 'during', 'until',
        'after', 'before', 'once'
    ]

    def __init__(self):
        """Initialize intent classifier."""
        logger.info("Intent classifier initialized")

    def classify(self, prompt: str, context: Dict[str, Any] = None) -> Dict[str, Any]:
        """Classify user intent from prompt.

        Args:
            prompt: User's natural language prompt
            context: Optional context information

        Returns:
            Dictionary with:
                - intent: 'patch', 'execute', or 'unclear'
                - confidence: 0.0-1.0
                - reasoning: Explanation of classification
                - matched_keywords: List of keywords that influenced decision
        """
        prompt_lower = prompt.lower()

        # Initialize scores
        patch_score = 0.0
        execute_score = 0.0
        matched_keywords = {'patch': [], 'execute': []}

        # Check for patch keywords
        for keyword in self.PATCH_KEYWORDS:
            if keyword in prompt_lower:
                patch_score += 1.0
                matched_keywords['patch'].append(keyword)
                logger.debug(f"Found patch keyword: {keyword}")

        # Check for execute keywords
        for keyword in self.EXECUTE_KEYWORDS:
            if keyword in prompt_lower:
                execute_score += 0.7  # Slightly lower weight
                matched_keywords['execute'].append(keyword)
                logger.debug(f"Found execute keyword: {keyword}")

        # Check for temporal indicators combined with actions
        for temporal in self.TEMPORAL_PATCH:
            if temporal in prompt_lower:
                # Check if followed by action verbs (send, notify, update, etc.)
                action_pattern = f"{temporal}.*?(send|notify|update|create|delete|add|remove|trigger|execute)"
                if re.search(action_pattern, prompt_lower):
                    patch_score += 1.5  # Strong indicator of patch
                    matched_keywords['patch'].append(f"{temporal}+action")
                    logger.debug(f"Found temporal+action pattern: {temporal}")

        # Check for conditional patterns (strong patch indicator)
        conditional_patterns = [
            r'if .* then',
            r'when .* (send|notify|do|execute)',
            r'whenever .* (happens|occurs)',
        ]
        for pattern in conditional_patterns:
            if re.search(pattern, prompt_lower):
                patch_score += 2.0
                matched_keywords['patch'].append('conditional_pattern')
                logger.debug(f"Found conditional pattern: {pattern}")

        # Check for question marks (strong execute indicator)
        if '?' in prompt:
            execute_score += 1.5
            matched_keywords['execute'].append('question')

        # Check for workflow modification verbs (patch indicators)
        modification_verbs = ['add', 'remove', 'delete', 'modify', 'change', 'update', 'insert', 'create']
        for verb in modification_verbs:
            # Check if verb is about modifying workflow structure
            if re.search(rf'\b{verb}\b.*(node|edge|step|notification|alert|email|webhook)', prompt_lower):
                patch_score += 1.5
                matched_keywords['patch'].append(f'{verb}_workflow')
                logger.debug(f"Found workflow modification: {verb}")

        # Determine intent based on scores
        total_score = patch_score + execute_score

        if total_score == 0:
            intent = 'unclear'
            confidence = 0.0
            reasoning = "No clear indicators found in prompt"
        elif patch_score > execute_score:
            intent = 'patch'
            confidence = min(patch_score / (total_score + 1), 1.0)
            reasoning = f"Detected permanent change indicators (score: {patch_score:.1f} vs {execute_score:.1f})"
        else:
            intent = 'execute'
            confidence = min(execute_score / (total_score + 1), 1.0)
            reasoning = f"Detected one-time operation indicators (score: {execute_score:.1f} vs {patch_score:.1f})"

        result = {
            'intent': intent,
            'confidence': confidence,
            'reasoning': reasoning,
            'matched_keywords': matched_keywords,
            'scores': {
                'patch': patch_score,
                'execute': execute_score
            }
        }

        logger.info(f"Intent classified: {intent} (confidence: {confidence:.2f})")
        return result

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
