"""Makes each pack importable (e.g. `import ecomops`) during tests without a
full pip install — mirrors connectors/conftest.py.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
