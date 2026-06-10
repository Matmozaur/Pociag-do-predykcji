.PHONY: format lint typecheck test check

format:
	uv run --extra dev ruff format src tests
	uv run --extra dev ruff check src tests --fix

lint:
	uv run --extra dev ruff check src tests

typecheck:
	uv run --extra dev mypy src

test:
	uv run --extra dev pytest tests -v

check: lint typecheck test
