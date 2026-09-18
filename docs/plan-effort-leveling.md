# Plan: Effort-Leveling (the second axis)

**Status**: 📋 Planned — design only. **No code exists.** Nothing in this document is
implemented, and the measurements cited are all from the
[Capability Leveling design record](plan-capability-leveling.md), not from this work.
**Author**: Josh Schoen
**Date**: 2026-09-18
**Loom version at writing**: v1.4.0

## Goal

Capability leveling ships one axis: **model identity**, with tier derived from catalog
pricing. This plan adds a second axis — **effort**, meaning how much work is spent per
attempt regardless of which model answers — and, critically, defines the boundary between
the two axes *before* either is wired to the other.

**Precise claim (do not overclaim):** this document contains no new measurement. It
re-reads the existing record, names the controls involved, and fixes the boundaries
between them so that each can be built and measured independently. Its deliverable is a
set of invariants and a falsifiable first experiment, not a feature.

**What triggered it:** the shipped ladder's headline mechanism — escalation to a stronger
model — fired once in 20 trials (Phase 2b) and zero times in 30 (Phase 5). Every measured
gain came from the cheap rung. If the cheap rungs are where the value is, they deserve to
be first-class controls rather than a side effect of the model ladder.

## Table of Contents

- [Influence is not coupling](#influence-is-not-coupling)
- [The controls](#the-controls)
- [The information-gain principle](#the-information-gain-principle)
- [Pairwise interaction matrix](#pairwise-interaction-matrix)
- [The scope mismatch](#the-scope-mismatch)
- [Coupling hazards](#coupling-hazards)
- [Prerequisites](#prerequisites)
- [The orthogonality test](#the-orthogonality-test)
- [Invariants](#invariants)
- [Build order](#build-order)
- [Phase 0 — the zero-cost replay](#phase-0--the-zero-cost-replay)
- [Deferred decisions](#deferred-decisions)
- [Risks](#risks)
- [Verification record](#verification-record)
- [See Also](#see-also)

## Influence is not coupling

The distinction this whole document rests on:

- **Influence** — one control's *value* changes another's *effect*. A fact about the
  world. Unavoidable, and worth keeping: it is the thing eventually measured and exploited.
- **Coupling** — one control's *setting* cannot be expressed, changed, or measured without
  the other. A design choice. Structural. It is what destroys the ability to observe
  influence.

The goal is not to separate these controls' effects; that is impossible and undesirable.
It is to keep every influence between them **observable** instead of baked into a name, a
key, or a composite dial.

The record already shows what the alternative costs. Phase 2b flagged its own confound —
"llama2 (7B) vs llama3.1 (8B) is a generation gap in instruction tuning, not a
parameter-count gap" — because two variables moved at once. Any design that encodes effort
into model or agent identity manufactures that same confound by construction, permanently.

## The controls

| | Control | What it buys | Scope today | Available on weak/local models? |
|---|---|---|---|---|
| **A** | Model identity | a different model answers | provider pool / role LLMs | — |
| **B1** | In-model thinking | more reasoning inside one LLM call | **nowhere** (dead knob) | ❌ reasoning models only |
| **B2** | Loop depth | more tool/observe iterations within one attempt | agent (`BehaviorConfig`) | ✅ |
| **C** | Retry with feedback | another attempt carrying failure information | **stage** (`retry_policy`) | ✅ |
| **D** | Learning | better prompt content on *future* runs | agent (`PatternConfig`) + learning DB | ✅ |

Two **inputs**, which are not controls:

| | Input | When | Cost | Shipped? |
|---|---|---|---|---|
| **V** | Verdict class (schema / execution / plan / judge) | post-flight | free…paid | ⚠️ schema only |
| **X** | Pre-flight complexity estimate (join depth, code metrics) | pre-flight | free | ❌ |

And **Bounds** (`max_escalations`, `max_cost_usd`), which are cross-cutting.

Related shipped surfaces that are adjacent but distinct:

- `IterativeWorkflowPattern.max_iterations` (default 3) plus `RestartPolicy` is loop depth
  at **workflow** scope — stages restarting earlier stages, not iterations within an attempt.
- Loom's "self-correction" (`Agent.executeToolWithSelfCorrection`) is **tool-level
  guardrails and circuit breakers**, not answer revision. There is no answer-revision loop
  in Loom today, so "learning cycles" has no existing implementation to inherit. The name
  is already taken by something else; reusing it would mislead.

## The information-gain principle

Re-reading the five existing experiments by *what the loop imports* rather than by what it
is called:

| Currency | Imports information from outside the model? | Measured (source phase) |
|---|---|---|
| Retry carrying the sqlite error text | **yes** | 17/30 → 23/30 (+20pp), all 8 execution failures resolved (Phase 5) |
| Retry carrying schema feedback | **yes** | 5/10 → 10/10 schema pass (Phase 2b) |
| Tool/retrieval iterations (BM25 recall) | **yes** | llama2 **0/30 → 30/30**, where deepseek-r1 alone scored 0/30 (Phase 4) |
| Self-critique | no | **0/18** wrong answers repaired (Phase 3) |
| Forced-step scaffolding | no | 27% (light) and 3% (heavy) against a 40% baseline (Phase 3b) |

The split is clean and it is not about loop count:

> **Information-bearing loops pay. Introspective loops fail or backfire.**

A loop that calls a tool, runs a query, or reads an error was worth more than a frontier
model (Phase 4: the weak model with retrieval beat the reasoning model without it). A loop
that re-reads its own output was worth less than nothing (Phase 3b: monotonically worse
with more imposed structure).

This reclassifies **B1** honestly. In-model thinking is introspective *by shape*, but it is
a **trained** mechanism rather than a prompted one, which is a different thing from
self-critique. The existing record cannot adjudicate it — B1 is simply **untested in Loom**.
That makes it the speculative currency, not the flagship, and it is the reason B1 is last in
the build order below rather than first.

It also gives the first principled *hypothesis* for ladder ordering, where previously there
was none: **order rungs by information gain, not by price.** That is a claim the record
supports and that the Phase 0 experiment can begin to test.

## Pairwise interaction matrix

| Pair | Do they influence each other? | Coupling risk | What keeps them separate |
|---|---|---|---|
| **A × B1** | Strongly. B1 exists only for some A, and its cost is A's output-token rate | **High** — identity fusion (`anthropic-high`); tier drifting toward effective price | Capability predicate + one cost function. **Tier stays a function of the model alone** |
| **A × B2** | **Measured as near-independent.** Phase 4: whenever the gold fact was in the prompt, the weak model used it — **75/75 across every arm** | **Medium-high** — agent identity fusion (`worker-deep`), because B2 is agent-scoped | A stage-scoped path to loop depth that is not a second agent |
| **A × C** | Asymmetric, and measured: C repairs format and execution failures on weak models (+20pp) and does not repair reasoning (0/18) | **Already shipped** — `tier_policies.<tier>.retry_budget` keys C off A's *price* | C should key on **V**, not A |
| **B1 × B2** | Unknown. Both spend output tokens; a thinking model may need fewer loops | **Low structurally, high via a composite dial** | Each currency individually addressable and individually recorded |
| **A × D** | Learned patterns are prompt content, and weak models use prompt content (75/75 again) | **Medium** — keying learned artifacts per-model fragments the corpus M ways | Key learning by task/domain; record model as a *dimension*, never a partition key |
| **Bounds × all** | All currencies must become commensurable in USD and wall-clock | **Legitimate join** | One cost function, one place — but see [Prerequisites](#prerequisites) |

The standout is **A × B2**. Phase 4's context-utilization finding — "whenever the gold fact
was in the prompt, the weak model used it, 75/75 across every arm" — is direct evidence that
loop depth's effect does *not* depend much on model strength. That is the empirical case for
making B2 the primary independent axis, and it is why "more thinking regardless of model" is
the right instinct rather than a hopeful one. It is also the cheapest interaction to measure,
because neither arm needs new plumbing.

## The scope mismatch

Of six effort currencies, **one is stage-scoped**. The leveling executor makes its decisions
at stage scope, and nearly everything it would want to spend lives at agent or workflow scope.

`PipelineStage` (`proto/loom/v1/orchestration.proto:91`) carries exactly eight fields:

| Field | # | Kind |
|---|---|---|
| `agent_id` | 1 | who |
| `prompt_template` | 2 | what |
| `validation_prompt` | 3 | verdict (legacy) |
| `retry_policy` | 4 | **control C** |
| `output_schema` | 5 | verdict (legacy) |
| `output_policy` | 6 | verdict |
| `hitl_gate` | 7 | human gate |
| `leveling_policy` | 8 | **control A** + bounds |

There is no loop-depth field, no thinking field, and no learning field. So this is not "add
an effort knob" — it is a scope question, with three possible answers:

1. Push the knobs **down** to stage scope (new per-stage override fields).
2. Pull the leveling decision **up** to agent scope.
3. Let leveling **request** effort without owning the knob.

Option 3 is the decoupled one and is what the invariants below assume: leveling owns the
verdict, the bounds, and one request — *spend more*. A resolver owns turning "more" into
whatever currencies this `(agent, model)` pair actually has. Capability discovery is answered
at runtime per pair, the way `SupportsStreaming` already works
(`pkg/types/types.go:234`), rather than guessed from config.

## Coupling hazards

Four identified so far. Three are hypothetical; one is already shipped.

### 1. Identity fusion, at three levels

Encoding effort into any identity string is the primary hazard, and it recurs at every level:

| Shortcut | What it fuses | Why it is worse than it looks |
|---|---|---|
| `provider: anthropic-high` | effort into **provider identity** | Propagates into pool keys, catalog lookup keys, tier input, and the `leveling_rung_model` metadata value |
| `model: claude-x-thinking` | effort into **model identity** | Same, plus it breaks catalog pricing lookups |
| `agent_id: worker-deep` | effort into **agent identity** | Worst case: agent identity also carries tools, prompts, memory and session, so five things vary to test one |

### 2. The composite dial

A single `effort: high` that silently expands B1 **and** B2 **and** C is itself a coupling.
Once shipped, "did high effort help?" is unanswerable, because no arm can tell which currency
moved. This is a hazard in the resolver design proposed above, not an argument against it.

Resolution: the resolver stays as a *default* for ergonomics, but (a) each currency remains
individually addressable in config, and (b) the resolver's expansion is recorded **per
currency** in telemetry, so any arm can be reconstructed after the fact. A composite dial is
acceptable as an ergonomic shortcut; it is not acceptable as the only representation.

### 3. Retry keyed on price (already shipped)

`tier_policies.<tier>.retry_budget` derives control **C** from axis **A** — how many times
you retry comes from what the model *costs*. The evidence says C's effectiveness is a
function of the **failure class** instead: format and execution failures repaired at +20pp
and 5/10 → 10/10, while reasoning failures resisted retry (0/18), critique (0/18) and
scaffolding (−13pp, −37pp) alike.

This is the same mistake one size smaller, and it is in `main` today. It is not urgent — the
knob works and its default is sane — but it shows the decoupling question is not
hypothetical here. Re-keying C on **V** is the first item in the build order.

### 4. Tier absorbing effort

If effort fed tier derivation (because effort changes effective price), short-circuiting
would become non-deterministic: the same model would take the active path or the
short-circuit path depending on a knob set elsewhere. Tier must stay a function of the model
alone. Stated as invariant 2.

## Prerequisites

Three things are true today that block the work, independent of any design choice.

### You cannot bound what you cannot meter

`docs/reference/workflow-leveling.md` states that per-attempt cost is read from the result
the attempt returned, which carries the **final LLM response's** usage, and that "an attempt
that ran a tool loop inside the agent reports less than it spent, so the gate can let a call
through that precise accounting would have blocked."

So the currency most worth spending (**B2**, loop depth) is precisely the one `max_cost_usd`
can least see. Leaning on it makes an already-understated gate worse, silently. **Per-currency
metering is a prerequisite for using loop depth as a rung at all** — bounds are the only thing
between "spend more" and unbounded spend.

### `Agent.SetProviderPool` has no production callers

`Agent.GetProviderPool()` (`pkg/agent/agent.go:4466`) is how `resolveLevelingLadder` resolves
a `provider:` rung, and `Agent.SetProviderPool` (`:4515`) is called **only from tests**.
`cmd/looms/cmd_serve.go:2565` injects the pool into the *registry*, which uses it to pick an
agent's main LLM (`pkg/agent/registry.go:1115`), and `:2558` injects it into the *server* —
neither pushes it onto the `Agent`. A ladder-resolution failure is returned as an error from
`executeStageWithLeveling` (`pkg/orchestration/pipeline_executor.go:812`), which fails the
stage and therefore the workflow.

Consequence: **`provider:`-based rungs cannot resolve in any production path today.**
`role:`-based rungs do work — role LLMs are wired from agent YAML by the registry
(`pkg/agent/registry.go:628-678`) and resolved strictly (`Agent.GetLLMForRoleStrict`,
`pkg/agent/agent.go:4238`). The shipped example
(`pkg/orchestration/testdata/leveling-pipeline.yaml`) uses a `provider:` rung and passes only
because the test calls `SetProviderPool` itself.

### `thinking_level` is a dead knob

`PresetDefaults.thinking_level` (`proto/loom/v1/templates.proto:114`, values
`"none" | "low" | "medium" | "high"`) is set by six presets in `pkg/templates/presets.go` and
written into the agent YAML `spec` by `pkg/shuttle/builtin/agent_management_templates.go:293`.
`pkg/agent/config_loader.go` never parses it. An agent created from a preset that claims
`thinking_level: high` gets no thinking.

This is the same class of thing the capability-leveling PR deleted two proto fields to avoid
(`scaffolding_depth`, `aggressive_coercion`, both now `reserved`). It must be **wired or
removed** before any B1 work, and in either case a second effort vocabulary must not be
invented next to it.

Also worth noting: `types.LLMProvider.Chat(ctx, messages, tools)` (`pkg/types/types.go:208`)
carries **no per-request options at all** — temperature and max-tokens are client-construction
parameters (`pkg/llm/factory/factory.go:223`). No provider sets a thinking budget or reasoning
effort anywhere; Bedrock and OpenAI only read reasoning content *back* into
`LLMResponse.Thinking`. B1 therefore needs an optional capability interface (the
`HealthChecker` / `StreamingLLMProvider` pattern already in `pkg/types/types.go`) rather than a
change to an interface with eight-plus implementations.

## The orthogonality test

The operational form of "do not couple them early". Apply to any future proposal on this
feature. Any **no** on 1–4, or **yes** on 5, means something has been fused:

1. **Config** — can I change control X without editing anything that names control Y?
2. **Measurement** — can I run an arm that varies X with Y held fixed?
3. **Telemetry** — can I read X's value from a trace without parsing Y's?
4. **Independence** — can I turn X off entirely and leave Y working?
5. **Identity** — does X appear in any identity string (provider, model, agent, pool key,
   catalog key)? This one must be **no**.

Worked results:

| Proposal | Fails |
|---|---|
| `provider: anthropic-high` | 2, 3, 5 |
| `agent_id: worker-deep` | 1, 2, 3, 4, 5 |
| Single composite `effort: high` with no per-currency record | 2, 3 |
| `tier_policies.<tier>.retry_budget` (shipped) | 2 — retry budget cannot be varied independently of tier |

## Invariants

Cheap to hold today, and each one preserves an option that is expensive to recover later.

1. **Effort never enters model identity** — not provider names, model strings, pool keys,
   catalog keys, or the `leveling_rung_provider` / `leveling_rung_model` metadata.
2. **Tier stays a function of the model alone.** Effort must not feed tier derivation.
3. **`leveling.model` and `leveling.effort` are independent span attributes**, so "did effort
   help, holding model fixed" is answerable from traces alone.
4. **Each axis is independently switchable** — effort with no ladder, ladder with no effort.
5. **One joint cost function, one place.** It is the only sanctioned place the axes meet,
   alongside the capability predicate and (once measured) the ordering policy.
6. **Effort never enters agent identity** — no `worker-deep`.
7. **Leveling requests effort; the owner of each knob realizes it.** Leveling never holds a
   loop-depth or pattern-recall dial, and every realization is recorded per currency.
8. **Learning is downstream of leveling, never a rung.** One direction, report-shaped.

On invariant 8: a cross-run horizon cannot help the stage that is failing now, so learning
cannot be a rung even in principle. The relationship inverts — **leveling emits the training
data; learning consumes it.** The leveling report already carries what a learning cycle wants
(tier, escalations, winning rung, pass/fail, budget exhausted, warnings), and
`Orchestrator.RecordPatternUsage` (`pkg/patterns/orchestrator.go:410`) already feeds an
effectiveness tracker. Nothing connects them. Keeping the direction one-way is what keeps the
measurement recoverable; as a rung, the coupling would be circular.

## Build order

Derived from coupling risk, not from expected payoff.

| # | Control | Why here | Blocked on |
|---|---|---|---|
| 1 | **C** — re-key retry on verdict class | Already stage-scoped and shipped; the only change is keying it on **V** instead of tier | Nothing |
| 2 | **B2** — loop depth as a requestable currency | Measured as the most model-independent currency, and works on every model | Per-currency metering; a stage-scoped path that is not a second agent |
| 3 | **V** — richer free verdict signals (plan-shape / EXPLAIN) | Free, post-flight, attacks the measured 83.3% ceiling; needs no effort plumbing at all | Nothing (independent of 1–2) |
| 4 | **X** — pre-flight complexity routing | The only thing that reaches failures no post-flight signal can see | Phase 0 below |
| 5 | **B1** — in-model thinking | Highest coupling risk on every axis, untested mechanism, and unavailable on exactly the models this feature targets | Dead-knob resolution; optional capability interface; `SetProviderPool` gap |
| — | **D** — learning | Stays downstream of the report, never a rung | — |

Build outward from the least-entangled control, and let each one earn the next by staying
independently measurable.

## Phase 0 — the zero-cost replay

**Purpose:** falsify the pre-flight axis (**X**) before designing anything around it. Zero
model calls, zero dollars, committed data only.

Phase 5 recorded two facts that, taken together, make this testable on data already in the
repo:

1. Failure structure was **bimodal by join depth** — filter/count and 2-table joins near
   perfect, **3-table joins 0/6**, top-N 0/6.
2. **7 silently-wrong queries executed cleanly, drew zero escalations, and deepseek-r1 solved
   all 7** — but the ladder never sent them. 83.3% was named as the structural ceiling of an
   execution-only signal.

So: the thing that predicted failure is knowable **pre-flight**, for free, and the failures it
predicts are exactly the ones no free post-flight signal can see.

**Method.** For each of the 30 questions in `docs/experiments/sql_questions.jsonl`, compute a
predictor from the question and schema alone — required join depth, with table/predicate counts
as secondary features — with no access to the generated SQL or the outcome. Score it against the
per-trial outcomes recorded in `docs/experiments/sql_arms.jsonl` for **arm 3** (the leveled arm
whose escalation decisions are being second-guessed), using the label *"should have escalated"* =
`silent_wrong`.

**Target class, stated precisely.** The predictor's target is the **retry-unfixable** failures.
Phase 5 attributed the top-N class (0/6) to "one deterministic syntax bug, retry-recoverable",
and retry did recover it; the 3-table-join class (0/6) was semantic and retry did not. The
predictor should separate the semantic class, not all failures.

**Rejection criteria — fixed before running.** The pre-flight axis is **rejected** unless the
predictor recovers at least **5 of the 7** silent-wrong trials while flagging no more than **1 in
3** of the 23 non-silent-wrong trials. Anything weaker is not a routing signal; it is a coin
flip with extra steps, and routing on it would spend frontier-model calls on questions the weak
model already answers correctly.

**Confounds to state in the result, whatever it says.** N=30, one seeded draw, one schema, five
template families, SQLite rather than Teradata dialect. A predictor tuned on the same 30 trials
it is scored against is fitted, not validated — so the honest output of Phase 0 is a
*go/no-go plus an effect size*, never an accuracy claim. A positive result earns a fresh
question set, not a feature.

**Cost:** no LLM calls, no dollars, no Ollama, no new dependencies. Runnable on any machine that
has the repo.

## Deferred decisions

Each one is deliberately open, and each is cheap to keep open under the invariants above.

1. Whether effort escalation precedes model escalation. Requires the 2×2 — fix model vary
   effort, fix effort vary model. **Note the conflict this creates:** ordering by information
   gain and ordering by catalog cost now disagree, and which wins is the interesting experiment
   rather than a design choice to make on paper.
2. Whether pre-flight complexity (**X**) sets effort, model, or both.
3. Whether loop depth belongs at stage scope at all, or leveling should request it agent-side.
4. Whether in-model thinking (**B1**) is worth the plumbing, *given* that tool loops are the
   proven currency and are available on every model.
5. Whether the verdict class (**V**) selects which control to move. The record suggests it
   should — format failures were fixed by retry with zero escalations, reasoning failures were
   not fixable by retry at all — but a fused ladder cannot express it, so the question only
   becomes answerable after the controls are separate.
6. Whether learned artifacts are keyed by model or by task/domain. Default assumption:
   task/domain, with model recorded as a dimension.
7. Effort granularity: per-stage, per-rung, or both.

## Risks

| Risk | Mitigation |
|---|---|
| Shipping a knob that controls nothing (the `thinking_level` failure mode) | Provider plumbing before config surface, always. No YAML key lands before the mechanism it names |
| A composite `effort` dial destroying measurability | Per-currency addressability and per-currency telemetry, as invariant 7 requires |
| Unbounded spend via the currency the cost gate cannot see | Per-currency metering is a prerequisite for B2, not a follow-up |
| Repeating C2 — deciding something before generation based on a guess about difficulty | Phase 0 falsifies the predictor on committed data before any code. C2 was rejected *because* Phase 3b measured its hypothesis first; same order here |
| Overclaiming "more thinking = better" | The information-gain principle is the claim, and it comes with its counter-claim: introspective loops measured 0/18 and −37pp |
| Proto churn on `PipelineStage` | Decide the scope question (push down vs. request) before adding any field; a field added and removed costs a permanent `reserved` line |

**The C2 parallel deserves its own sentence.** Pre-flight effort selection shares a silhouette
with capability-adaptive scaffolding, which was ❌ rejected on evidence: imposed structure made
the weak model monotonically worse. It is genuinely a different mechanism — a model capability
knob, not a longer prompt — but it shares the shape "decide something before generation from a
guess about difficulty". The discipline that killed C2 cheaply is the discipline to reuse.

## Verification record

Design-only; no tests were run and none are claimed. What was verified in the v1.4.0 tree at
`4fa5742`, by reading the code:

- ✅ `PipelineStage` carries eight fields, none of them loop depth, thinking, or learning
  (`proto/loom/v1/orchestration.proto:91`).
- ✅ Loop depth is agent-scoped: `BehaviorConfig.max_iterations` (field 1, "tool call iterations
  per turn"), `max_turns` (5, default 25), `max_tool_executions` (6, default 50)
  (`proto/loom/v1/agent_config.proto:510`).
- ✅ Learning is agent-scoped: `PatternConfig.max_patterns_per_turn` (default 1),
  `min_confidence` (default 0.75), `enable_tracking`
  (`proto/loom/v1/agent_config.proto:546`); `Orchestrator.RecordPatternUsage`
  (`pkg/patterns/orchestrator.go:410`).
- ✅ `thinking_level` is parsed by nothing: set in `pkg/templates/presets.go`, written by
  `pkg/shuttle/builtin/agent_management_templates.go:293`, absent from
  `pkg/agent/config_loader.go`.
- ✅ `types.LLMProvider.Chat` takes no per-request options (`pkg/types/types.go:208`); optional
  capability interfaces `HealthChecker` and `StreamingLLMProvider` exist in the same file as
  precedent.
- ✅ `ModelInfo.is_reasoning` (field 10, `proto/loom/v1/loom.proto:2373`) is the available
  capability gate for B1.
- ✅ `Agent.SetProviderPool` (`pkg/agent/agent.go:4515`) has zero non-test callers; the registry
  and server pools (`cmd/looms/cmd_serve.go:2558,2565`) never reach the `Agent`.
- ✅ Role LLMs are wired in production (`pkg/agent/registry.go:628-678`) and resolved strictly
  (`pkg/agent/agent.go:4238`), so `role:` rungs work where `provider:` rungs do not.
- ✅ Loom's self-correction is tool-level (`Agent.executeToolWithSelfCorrection`,
  `pkg/agent/agent.go:3384`; `WithoutSelfCorrection`, `:602`), not answer revision.
- ✅ Cost is metered from the final LLM response's usage, so tool loops under-report — stated in
  `docs/reference/workflow-leveling.md` ("Cost Gate") and in the `LevelingPolicy.MaxCostUSD`
  doc comment.
- ✅ The Phase 0 inputs exist: `docs/experiments/sql_arms.jsonl` and
  `docs/experiments/sql_questions.jsonl`.
- ⚠️ No `leveling:` block appears in any shipped example workflow;
  `examples/reference/workflows/workflow-all-fields-reference.yaml` does not mention it. Worth
  fixing independently of this plan.
- 📋 Every measurement cited in this document comes from
  [plan-capability-leveling.md](plan-capability-leveling.md) (Phases 2b–5). No new measurement
  was performed.

## See Also

- [Capability Leveling design record](plan-capability-leveling.md) — the measured record this
  plan re-reads, including the C2 and C4 rejections
- [Workflow Capability Leveling Reference](reference/workflow-leveling.md) — the shipped model
  axis: tiers, ladder, bounds, failure semantics
- [Workflow Output Retry Reference](reference/workflow-output-retry.md) — control **C** as it
  exists today
