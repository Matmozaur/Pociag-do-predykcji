import marimo

__generated_with = "0.25.1"
app = marimo.App()


@app.cell
def _():
    import marimo as mo
    import pandas as pd
    import polars as pl

    return mo, pd, pl


@app.cell
def _(mo):
    mo.md("""
    # Hello, world!
    """)
    return


@app.cell
def _(pd):
    pd.DataFrame({"greeting": ["hello", "world"], "n": [1, 2]})
    return


@app.cell
def _(pl):
    pl.DataFrame({"greeting": ["hello", "world"], "n": [1, 2]})
    return


@app.cell
def _():
    return


@app.cell
def _():
    return


@app.cell
def _():
    return


@app.cell
def _():
    return


if __name__ == "__main__":
    app.run()
