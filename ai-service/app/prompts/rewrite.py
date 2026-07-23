from app.domain.llm import LLMMessage, LLMRequest


def build_rewrite_prompt(text: str, tone: str | None, locale: str | None) -> LLMRequest:
    normalized_tone = tone or "clear, concise, and natural"
    normalized_locale = locale or "the same language as the input"
    return LLMRequest(
        messages=[
            LLMMessage(
                role="system",
                content=(
                    "You rewrite chat text for the user. Preserve the original meaning, "
                    "avoid inventing facts, and return only the rewritten text."
                ),
            ),
            LLMMessage(
                role="user",
                content=(
                    f"Rewrite the following text in {normalized_locale} with a "
                    f"{normalized_tone} tone:\n\n{text}"
                ),
            ),
        ],
        temperature=0.2,
    )
