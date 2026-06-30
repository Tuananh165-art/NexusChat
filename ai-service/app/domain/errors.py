class AIServiceError(Exception):
    """Base error for AI service failures."""


class ProviderConfigurationError(AIServiceError):
    """Raised when the configured LLM provider is incomplete or invalid."""


class ProviderRequestError(AIServiceError):
    """Raised when the LLM provider fails to complete a request."""
