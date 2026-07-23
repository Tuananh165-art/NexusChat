FROM python:3.14-slim AS runtime

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

COPY pyproject.toml README.md ./
COPY app ./app
COPY migrations ./migrations
COPY alembic.ini ./alembic.ini

RUN pip install --no-cache-dir .

EXPOSE 8090

CMD ["sh", "-c", "export DATABASE_URL=postgresql+asyncpg://nexuschat_ai:${AI_POSTGRES_PASSWORD}@ai-postgres:5432/nexuschat_ai && python -m alembic upgrade head && uvicorn app.main:app --host 0.0.0.0 --port 8090"]
