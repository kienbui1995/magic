"""Makes each connector package importable (e.g. `import zalo`) during tests,
without requiring a full pip install — mirrors how this directory is expected
to eventually split into standalone per-connector packages/repos.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
