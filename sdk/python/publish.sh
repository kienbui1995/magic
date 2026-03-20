#!/bin/bash
set -e

if [ -z "$PYPI_TOKEN" ]; then
  echo "Error: PYPI_TOKEN is not set"
  exit 1
fi

rm -rf dist/
python -m build
twine upload dist/* -u __token__ -p "$PYPI_TOKEN"
