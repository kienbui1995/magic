# MagiC Benchmark Results

Each result file is tied to a specific MagiC release and scenario. File naming:

```
v<MAJOR>.<MINOR>.<PATCH>-<scenario>.md
```

## Template

Use the structure below when adding a new run. Do not overwrite prior files —
append new ones so regressions remain visible over time.

```markdown
# MagiC v<VERSION> — <scenario>

- **Run date:** YYYY-MM-DD
- **Git SHA:** <short-sha>
- **Hardware:** <CPU model, cores, RAM, disk>
- **Go:** go1.XX
- **Postgres:** 16 (local socket | docker)
- **Deviations from reference rig:** <none | describe>

## Methodology

Short restatement of the scenario + any deviations (e.g. "used 5 workers
instead of 10 because the test rig only has 4 cores").

## Results

| Metric | Value |
|--------|-------|
| ... | ... |

## Observations

Prose notes: GC pauses, saturation points, anomalies worth investigating.
```

## Note on synthetic numbers

Files containing the word **"baseline"** before an actual measured run are
placeholders with illustrative values — they describe the expected shape of
the output, not observed performance. Always check the file header for the
"synthetic / illustrative" disclaimer before quoting a number externally.
