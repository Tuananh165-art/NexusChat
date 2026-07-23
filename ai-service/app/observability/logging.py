import logging

from app.config.settings import Settings


def configure_logging(settings: Settings) -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    logging.getLogger("nexuschat.ai").info(
        "configured logging for %s in %s",
        settings.service_name,
        settings.environment,
    )
