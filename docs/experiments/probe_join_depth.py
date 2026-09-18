#!/usr/bin/env python3
"""Replay probe: does a pre-flight complexity signal separate the silent-wrong class?

Reads the committed Phase 5 data only. No model calls, no network, no dollars.

    python3 docs/experiments/probe_join_depth.py

Two outputs:

1. Per-arm aggregates recomputed from the raw per-trial rows, so the figures
   quoted in docs/plan-capability-leveling.md (Phase 5) can be checked against
   the data rather than taken from prose.

2. A pre-check of the Phase 0 predictor described in
   docs/plan-effort-leveling.md: flag a question as "likely to fail in a way
   retry cannot fix" from its required join depth, and score that flag against
   the recorded silent_wrong outcomes in the leveled ladder arm.

READ THIS BEFORE QUOTING (2) — the pre-check is deliberately the WEAK form of
the experiment, and it overstates the predictor twice:

  a. Join depth here is derived from `reference_sql`, i.e. from the gold answer.
     Production has no reference SQL; a usable predictor must be computed from
     the question plus the schema. This probe therefore establishes that join
     depth separates the failure class, NOT that an estimable predictor does.

  b. Join depth is perfectly collinear with template family in this question
     set: family 3 is the only depth-3 family, and it is exactly 6 of the 30
     questions. So "depth >= 3" and "family == 3" are the same predictor here,
     and the effective sample size for the signal is 5 families, not 30 trials.

Phase 0 proper has to fix both. See "Phase 0" in docs/plan-effort-leveling.md.
"""

import collections
import json
import pathlib
import re

HERE = pathlib.Path(__file__).resolve().parent
ARMS = HERE / "sql_arms.jsonl"
QUESTIONS = HERE / "sql_questions.jsonl"

# The synthetic retail schema Phase 5 generated (4 tables, 1118 rows).
TABLES = ("customers", "orders", "order_items", "products")

# Pre-registered in docs/plan-effort-leveling.md before this probe was run.
MIN_RECOVERED = 5
MAX_FALSE_FLAG_RATE = 1 / 3


def load(path):
    with path.open() as fh:
        return [json.loads(line) for line in fh if line.strip()]


def join_depth(sql):
    """Distinct schema tables named in a statement.

    Leaky by construction when called on reference_sql — see the module docstring.
    """
    lowered = sql.lower()
    return sum(1 for t in TABLES if re.search(rf"\b{t}\b", lowered))


def per_arm(rows):
    arms = collections.OrderedDict()
    for row in rows:
        arms.setdefault(row["arm"], []).append(row)

    print("=" * 94)
    print("PER-ARM AGGREGATES  (recomputed from raw rows)")
    print("=" * 94)
    print(
        f"{'arm':<22}{'correct':>8}{'silent':>8}{'exec_err':>9}"
        f"{'calls':>7}{'r1':>5}{'escal':>7}{'judge':>7}{'secs':>9}"
    )
    for arm, rs in arms.items():
        cat = collections.Counter(r["final"]["category"] for r in rs)
        print(
            f"{arm:<22}{cat['correct']:>8}{cat['silent_wrong']:>8}{cat['exec_error']:>9}"
            f"{sum(r['calls'] for r in rs):>7}"
            f"{sum(r['strong_model_calls'] for r in rs):>5}"
            f"{sum(r['escalations'] for r in rs):>7}"
            f"{sum(r['judge_calls'] for r in rs):>7}"
            f"{sum(r['seconds'] for r in rs):>9.1f}"
        )
    return arms


def models(rows):
    print()
    print("=" * 94)
    print("WHICH MODEL RAN AT WHICH RUNG  (every arm was local Ollama)")
    print("=" * 94)
    seen = collections.defaultdict(collections.Counter)
    for row in rows:
        for att in row["attempts"]:
            seen[row["arm"]][(att["rung"], att["model"])] += 1
    print(f"{'arm':<22}{'rung':>5}  {'model':<24}{'attempts':>9}")
    for arm in seen:
        for (rung, model), n in sorted(seen[arm].items()):
            print(f"{arm:<22}{rung:>5}  {model:<24}{n:>9}")
    print()
    print("  No cost field exists in these rows; the recorded currencies are")
    print("  `seconds` and `calls`. Ollama is priced 0/0 in the catalog, so the")
    print("  dollar economics of the ladder are untested by this experiment.")


def predictor(arms, questions):
    ladder = next(rs for arm, rs in arms.items() if arm.startswith("3"))

    print()
    print("=" * 94)
    print("OUTCOME BY DERIVED JOIN DEPTH  (ladder arm)")
    print("=" * 94)
    table = collections.defaultdict(collections.Counter)
    for row in ladder:
        depth = join_depth(questions[row["index"]]["reference_sql"])
        table[depth][row["final"]["category"]] += 1
    print(f"{'depth':>6}{'n':>5}{'correct':>9}{'silent':>8}{'exec_err':>10}")
    for depth in sorted(table):
        c = table[depth]
        print(
            f"{depth:>6}{sum(c.values()):>5}{c['correct']:>9}"
            f"{c['silent_wrong']:>8}{c['exec_error']:>10}"
        )

    families = collections.defaultdict(set)
    for q in questions.values():
        families[q["family"]].add(join_depth(q["reference_sql"]))
    counts = collections.Counter(q["family"] for q in questions.values())
    print()
    print("  depth by family (collinearity check):")
    for f in sorted(families):
        print(f"    family {f}: n={counts[f]:<3} depth={sorted(families[f])}")

    tp = fp = fn = tn = 0
    for row in ladder:
        flagged = join_depth(questions[row["index"]]["reference_sql"]) >= 3
        silent = row["final"]["category"] == "silent_wrong"
        if flagged and silent:
            tp += 1
        elif flagged:
            fp += 1
        elif silent:
            fn += 1
        else:
            tn += 1

    rate = fp / (fp + tn) if (fp + tn) else 0.0
    passed = tp >= MIN_RECOVERED and rate <= MAX_FALSE_FLAG_RATE

    print()
    print("=" * 94)
    print("PRE-REGISTERED CRITERIA  (weak form — see module docstring)")
    print("=" * 94)
    print(f"  silent_wrong trials      : {tp + fn}")
    print(f"  recovered                : {tp}   (criterion: >= {MIN_RECOVERED})")
    print(f"  missed                   : {fn}")
    print(
        f"  false flags              : {fp} of {fp + tn} = {rate:.1%}"
        f"   (criterion: <= {MAX_FALSE_FLAG_RATE:.1%})"
    )
    print(f"  formal verdict           : {'PASS' if passed else 'FAIL'}")
    print()
    print("  Escalations that fired on the silent_wrong trials:",
          sorted(r["escalations"] for r in ladder if r["final"]["category"] == "silent_wrong"))
    print("  Strong-model calls in the ladder arm:",
          sum(r["strong_model_calls"] for r in ladder))
    print()
    print("  A PASS here does NOT clear the pre-flight axis. The predictor was")
    print("  computed from the gold SQL and is collinear with template family,")
    print("  so it recognizes one family rather than generalizing. Effective N")
    print("  is 5 families, not 30 trials.")


def main():
    rows = load(ARMS)
    questions = {q["index"]: q for q in load(QUESTIONS)}
    arms = per_arm(rows)
    models(rows)
    predictor(arms, questions)


if __name__ == "__main__":
    main()
