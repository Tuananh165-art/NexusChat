from app.config.settings import Settings
from app.domain.llm import LLMProvider
from app.providers.openai_compatible import OpenAICompatibleProvider


def create_llm_provider(settings: Settings) -> LLMProvider:
    return OpenAICompatibleProvider(settings)
