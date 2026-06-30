.PHONY: test lint run

test:
	python -m pytest

lint:
	python -m ruff check .

run:
	python -m uvicorn app.main:app --host 0.0.0.0 --port 8090 --reload
