# Plan: Effort-Leveling (the second axis)

**Status**: 📋 Planned — design only. **No code exists**, and **no model was called for this
work.** The measurements cited are from the
[Capability Leveling design record](plan-capability-leveling.md). One replay analysis over
that record's committed data was run here (`docs/experiments/probe_join_depth.py`); its
result is reported in [Phase 0](#phase-0--the-zero-cost-replay) with the two reasons it
proves less than it appears to.
**Author**: Josh Schoen
**Date**: 2026-09-18
**Loom version at writing**: v1.4.0

## Companion documents

This record and the reader-facing explainer are two altitudes on one body of work, and
neither is derived from the other:

| Document | Altitude | Owns |
|---|---|---|
| **this record** — `docs/plan-effort-leveling.md` | how to build it | control boundaries, invariants, falsification criteria, evidence |
| **the explainer** — `docs/explainers/capability-leveling.html` | what it is and whether it works | the shipped mechanism, the measured results, the trade-offs, the diagrams |
| the reference — `docs/reference/workflow-leveling.md` | how to configure it | every key, default and error of the shipped surface |

Every idea in the explainer has a home here. That is the rule that keeps them from drifting
into two different features: the explainer may be rewritten freely, but nothing may exist
only there.

## Goal

Capability leveling ships one axis: **model identity**, with tier derived from catalog
pricing. This plan adds a second axis — **effort**, meaning how much work is spent per
attempt regardless of which model answers — and, critically, defines the boundary between
the two axes *before* either is wired to the other.

> [!IMPORTANT]
> **Precise claim — do not overclaim.** This document runs no new experiment. It re-reads the
> existing record, names the controls involved, and fixes the boundaries between them so that
> each can be built and measured independently. Its deliverable is a set of invariants and a
> falsifiable first experiment, not a feature. The one analysis it does perform is a replay over
> already-committed per-trial data, and it is reported as a pre-check that **amends** the
> experiment rather than as a result that clears it.

**What triggered it:** the shipped ladder's headline mechanism — escalation to a stronger
model — fired once in 20 trials (Phase 2b) and zero times in 30 (Phase 5). Every measured
gain came from the cheap rung. If the cheap rungs are where the value is, they deserve to
be first-class controls rather than a side effect of the model ladder.

## Table of Contents

- [Companion documents](#companion-documents)
- [Influence is not coupling](#influence-is-not-coupling)
- [The controls](#the-controls)
- [The information-gain principle](#the-information-gain-principle)
- [The check ladder](#the-check-ladder)
- [What the record does not measure](#what-the-record-does-not-measure)
- [Pairwise interaction matrix](#pairwise-interaction-matrix)
- [The scope mismatch](#the-scope-mismatch)
- [Coupling hazards](#coupling-hazards)
- [Prerequisites](#prerequisites)
- [The orthogonality test](#the-orthogonality-test)
- [The dial and its presets](#the-dial-and-its-presets)
- [Invariants](#invariants)
- [Build order](#build-order)
- [Phase 0 — the zero-cost replay](#phase-0--the-zero-cost-replay)
- [Deferred decisions](#deferred-decisions)
- [Risks](#risks)
- [Verification record](#verification-record)
- [Rendering this record as a page](#rendering-this-record-as-a-page)
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
| **S** | Checking depth | a verdict that can see more kinds of wrong | **stage** (`output_policy`), schema only | ✅ |

**S** was an input in the first draft of this record and is a control in this one. How deep
you check is something you *buy*, on its own ladder, and getting it wrong is what produced
the only ceiling the program actually measured. See [The check ladder](#the-check-ladder).

Two **inputs**, which are not controls:

| | Input | When | Cost | Shipped? |
|---|---|---|---|---|
| **V** | Verdict *class* the check returns (format / execution / plan / meaning) | post-flight | — | ⚠️ not carried |
| **X** | Pre-flight complexity estimate (join depth, code metrics) | pre-flight | free | ❌ |

**S** and **V** are easy to confuse and must not be: **S** is how much checking you paid
for, **V** is what the check came back and said. **S** is a dial; **V** is a label that
should travel with the failure so **C** can key on it.

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

## What the record does not measure

The Phase 5 figures reproduce exactly from the committed per-trial rows
(`docs/experiments/probe_join_depth.py`):

| Arm | Correct | Silent wrong | Exec error | Calls | Strong-model calls | Escalations | Judge calls | Seconds |
|---|---|---|---|---|---|---|---|---|
| 1 llama3.2, leveling off | 17 | 5 | 8 | 30 | 0 | 0 | 0 | 25.3 |
| 2 + retry on exec error | 23 | 7 | 0 | 38 | 0 | 8 | 38 | 30.4 |
| 3 + ladder → r1 | 23 | 7 | 0 | 38 | **0** | 8 | 38 | 29.7 |
| 4 r1 alone (ceiling) | 30 | 0 | 0 | 30 | 30 | 0 | 0 | 475.2 |

The 16× wall-clock figure is 475.2 / 29.7 from those `seconds` fields. The 38 judge calls per
leveled arm are the free sqlite execution checks (30 + 8 retries). One detail the prose did not
carry: escalations on the seven silent-wrong trials were `[0, 0, 0, 0, 0, 1, 1]` — **two of
them did fire a rung**, and those are the two that were execution errors in arm 1 and became
confidently wrong after retry. The data confirms retry manufactured them.

Three limits follow, and they bound what this plan may assume.

### Every arm was a local Ollama model

`llama3.2:latest` at rung 0 in arms 1–3, `deepseek-r1:latest` at rung 0 in arm 4. Nothing else
ran.

### The model axis has never been exercised across tiers

In **both** leveled arms the rung-1 model was `llama3.2:latest` — the same model as rung 0. The
r1 rung existed in arm 3's ladder and was never reached. And under the shipped tier rules
`deepseek-r1:latest` classifies as tier **`local`**, not `frontier`: rule 2 (zero pricing) is
applied before rule 3 (the reasoning flag), deliberately, because a self-hosted reasoning model
is still free to retry. So even had that rung fired it would have been local → local.

> [!WARNING]
> **The stronger statement.** "Escalation did not fire" is the weaker of the two available.
> **No measurement in the record covers a local → frontier escalation** — the model axis has
> never been measured across a tier boundary at all.

### The dollar economics are untested on both sides

There is **no cost field in the per-trial rows**. The recorded currencies are `seconds` and
`calls`, and Ollama is priced 0/0 in the catalog, so every arm cost $0 — Phase 2b says this
plainly ("demonstrates no dollar saving and says nothing about one") and Phase 5 inherits it.

The consequence for this plan is specific: **deferred decision 1 (order rungs by information
gain, or by catalog cost?) has no supporting data on either side.** That does not weaken the
build order, it sharpens it — ordering by information gain has evidence behind it, and ordering
by cost has none yet.

One further consequence for the effort axis: neither of these models accepts a thinking budget,
so **B1** could not have been tested here even if it were wired. That is not a gap in the
experiment; it is the structural point of this plan, with the models named. `B2` and `C` worked
on both.

## The check ladder

📋 **Designed, not built.** The shipped verdict is a JSON Schema from config, or an
execution check written in Go for the benchmark. Everything below is a proposal.

The measured 83.3% ceiling is not a law about weak models. It is a consequence of asking a
database the cheapest possible question — "did it run" — when a database can answer much
better ones, most of them without reading a row.

| # | Check | Cost class | Catches what the cheaper rungs cannot |
|---|---|---|---|
| 1 | Parse | no DB | Syntax errors, before touching anything |
| 2 | DDL / schema | metadata only | Invented columns, wrong types, join keys that are not keys |
| 3 | EXPLAIN | optimizer only | Names that bind but a plan that is wrong: product joins, unexpected full scans, no-confidence estimates |
| 4 | DBQL history | one log query | A query shape never run on this system, or an estimate far outside what this workload does |
| 5 | Execute | reads data | Runtime failures the optimizer accepted |
| 6 | Verify the result | one model call | Answers that are wrong but ran cleanly |

Rung 5 is the only one measured so far, and on its own it was worth +20 points. Rungs 2–4
are the missing ones, and they are missing on the cheap side of the ladder.

### Why EXPLAIN is the load-bearing rung

The 7 silent failures in Phase 5 were wrong joins and invented arithmetic. A wrong join is
visible in a plan — as a product join, or a scan nobody asked for — **without executing
anything**. That makes rung 3 strictly stronger than rung 5 and strictly cheaper, which is
the first time in this program that a signal has been available on both counts. The
optimizer also reports its own confidence, and a plan built on no-confidence estimates is
frequently a query that is not asking what its author meant.

It is also safe in a way that matters structurally: it reads no data and mutates nothing.
Escalation rungs run without tools, so a check that needs no tool call fits in places a
repair cannot.

### What exists to build it on

| Primitive | State |
|---|---|
| `fabric.GetSchema` returning field name, type, nullability, primary key | ✅ shipped — rung 2 needs no new plumbing |
| `fabric.ExecuteQuery` (an `EXPLAIN …` is just a query) | ✅ shipped |
| `Capabilities.Features` / `SupportedOperations` — how a backend advertises EXPLAIN or DBQL reachability | ✅ shipped — this is what "where available" resolves against |
| An EXPLAIN **plan parser** | ❌ absent. `pkg/mcp/apps/html/explain-plan-visualizer.html` is a display surface registered as an MCP resource, with nothing in Go producing plan data for it — a renderer, not a parser |
| DBQL as a verification baseline | ❌ absent. DBQL appears only as domain knowledge inside Teradata performance patterns, for *analysing* a system |

### Termination is part of the design

A ladder of checks that does not know when to stop is a loop that spins. The rule:

> When every check available on this backend passes and the answer is still wrong, the loop
> is out of information. Escalate the model, or hand it to a human. Do not re-check.

That is the honest end of the free-signal path, and it is why **S** has a ceiling like every
other control rather than running until something breaks.

### The caveat that survives all of this

Rungs 1–4 verify that a query is **well-formed and plausible**, not that it answers the
question that was asked. Only rung 6 sees meaning, and it costs a model call — which makes
the top of this ladder a cost decision, not a capability one. Phase 3b measured what that
costs: a reasoning model as critic agreed 20/20, at ~31s per verdict against ~18s to
generate the answer in the first place.

## Pairwise interaction matrix

| Pair | Do they influence each other? | Coupling risk | What keeps them separate |
|---|---|---|---|
| **A × B1** | Strongly. B1 exists only for some A, and its cost is A's output-token rate | **High** — identity fusion (`anthropic-high`); tier drifting toward effective price | Capability predicate + one cost function. **Tier stays a function of the model alone** |
| **A × B2** | **Measured as near-independent.** Phase 4: whenever the gold fact was in the prompt, the weak model used it — **75/75 across every arm** | **Medium-high** — agent identity fusion (`worker-deep`), because B2 is agent-scoped | A stage-scoped path to loop depth that is not a second agent |
| **A × C** | Asymmetric, and measured: C repairs format and execution failures on weak models (+20pp) and does not repair reasoning (0/18) | **Already shipped** — `tier_policies.<tier>.retry_budget` keys C off A's *price* | C should key on **V**, not A |
| **B1 × B2** | Unknown. Both spend output tokens; a thinking model may need fewer loops | **Low structurally, high via a composite dial** | Each currency individually addressable and individually recorded |
| **A × D** | Learned patterns are prompt content, and weak models use prompt content (75/75 again) | **Medium** — keying learned artifacts per-model fragments the corpus M ways | Key learning by task/domain; record model as a *dimension*, never a partition key |
| **S × A** | **This is the pairing that matters.** A deeper check is what tells the model ladder when to fire; on an execution-only check the ladder could not see the 7 failures the strong model would have fixed | **Low** — they are different kinds of thing | Nothing to do beyond not conflating **S** with **V** |
| **S × C** | Measured: the check's *payload* is what makes retry work at all (+20pp with the sqlite error text, 0/18 with a critique) | **Already shipped** — `retry_budget` keys off tier price, not the verdict class | Carry **V** with the failure, then key **C** on it |
| **S × B2** | A check that runs through a tool spends the agent's tool budget | **Medium** — a deeper check could silently starve the loop it shares a budget with | Check cost is metered as **S**, never charged to **B2** |
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

> [!WARNING]
> `docs/reference/workflow-leveling.md` states that per-attempt cost is read from the result
> the attempt returned, which carries the **final LLM response's** usage, and that "an attempt
> that ran a tool loop inside the agent reports less than it spent, so the gate can let a call
> through that precise accounting would have blocked."

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

## The dial and its presets

📋 **Proposed.** The model for the effort control is a graphic equalizer: five bands, and
named presets that are nothing more than a remembered position of those same sliders.

| Band | Control | State today |
|---|---|---|
| model tier | **A** | ✅ shipped |
| thinking | **B1** | ❌ named, never read |
| loops | **B2** | ⚠️ agent scope only |
| retries | **C** | ✅ shipped |
| checks | **S** | ⚠️ schema only |

Three placeholder presets, each a real configuration rather than a mood:

| Preset | Shape | Why |
|---|---|---|
| `Draft` | low on everything | Wrong answers are acceptable at this stage |
| `Contract` | cheap model, no thinking, **many retries**, moderate checks | The configuration the SQL benchmark actually validated |
| `Audit` | **maximum checks**, high loops, mid model | Silent wrongness is the thing being bought out |

Two requirements, both following directly from invariant 7:

1. **A preset is a position, not a behaviour.** Every band stays individually addressable,
   and the position a preset set is recorded per band. A dial that moves three currencies
   without saying so makes the next benchmark unable to attribute its own result — which
   would destroy exactly the property that made every finding in this document legible.
2. **Names describe the workload, not the sound.** Someone choosing a preset needs to know
   what it does to their bill and their error rate. `Draft` / `Contract` / `Audit` at least
   say that much; the final names are an open question.

The band layout makes one thing plain that prose kept fumbling: the leftmost slider is a
different *kind* of thing from the other four. Moving it changes who answers. Moving any of
the others changes how hard the same model works.

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

9. **The check ladder terminates.** When every check available on the backend passes and the
   answer is still wrong, the free signal is exhausted: escalate the model or hand it to a
   human. Never re-check in a loop.
10. **A check declares its cost class** — no DB, metadata, optimizer, log query, reads data,
    model call — so the ladder can order itself cheapest-first and the gate can meter it.
    Check spend is attributed to **S**, never charged to the tool-loop budget it may share.

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
| 1 | **S** — the check ladder, rungs 2–4 (DDL, EXPLAIN, DBQL) | Free or near-free, attacks the one ceiling that was actually measured, needs no effort or model plumbing at all, and is the one mechanism that never failed to fire | An EXPLAIN plan parser; `Capabilities.Features` gating |
| 2 | **C** — re-key retry on verdict class | Already stage-scoped and shipped; the only change is keying it on **V** instead of tier — and a richer **S** is what makes **V** worth keying on | Carrying **V** with the failure (cheap once 1 lands) |
| 3 | **B2** — loop depth as a requestable currency | Measured as the most model-independent currency, and works on every model | Per-currency metering; a stage-scoped path that is not a second agent |
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
predictor from the **question text and the schema only** — estimated join depth, with table and
predicate counts as secondary features. The predictor may not read `reference_sql`, the generated
SQL, or the outcome. Score it against the per-trial outcomes recorded in
`docs/experiments/sql_arms.jsonl` for **arm 3** (the leveled arm whose escalation decisions are
being second-guessed), using the label *"should have escalated"* = `silent_wrong`.

The no-`reference_sql` rule is load-bearing rather than pedantic: production has no reference
SQL, so a predictor that reads it measures nothing that could ship. The pre-check below violated
this rule, which is how the rule got written down.

**Target class, stated precisely.** The predictor's target is the **retry-unfixable** failures.
Phase 5 attributed the top-N class (0/6) to "one deterministic syntax bug, retry-recoverable",
and retry did recover it; the 3-table-join class (0/6) was semantic and retry did not. The
predictor should separate the semantic class, not all failures.

> [!WARNING]
> **Rejection criteria — fixed before running, and restated per family.** Depth does not vary
> within a template family in this question set (see the pre-check), so a per-trial bar is
> satisfiable by recognizing one family. Phase 0 is therefore scored two ways, and must clear
> **both** of the bars below.

1. **Per family.** The predictor must separate the failing family from the passing families on
   features it could compute in production, across **at least 3 of the 5** families — i.e. it
   must not reduce to a single-family detector.
2. **Per trial.** At least **5 of the 7** silent-wrong trials recovered, with no more than **1 in
   3** of the 23 non-silent-wrong trials flagged.

Anything weaker is not a routing signal; it is a coin flip with extra steps, and routing on it
would spend frontier-model calls on questions the weak model already answers correctly.

⚠️ **Alternatively, and better: fix the question set instead of the scoring.** A question set
that varies join depth *within* a family breaks the collinearity at its source and makes the
per-trial bar meaningful on its own. That is the preferred route if a new generator run is
affordable; the two-way scoring above is the fallback when it is not.

### Preliminary pre-check (2026-09-18) — ⚠️ passes the old bar, and does not count

`docs/experiments/probe_join_depth.py`, run against the committed data:

| Derived join depth | n | Correct | Silent wrong |
|---|---|---|---|
| 1 | 18 | 17 | 1 |
| 2 | 6 | 6 | 0 |
| **3** | **6** | **0** | **6** |

Against the criteria as originally written: **6 of 7** silent wrongs recovered (bar: ≥ 5) and
**0 false flags out of 23** (bar: ≤ 33.3%). Formally a pass, with room. It does not clear the
pre-flight axis, for two reasons, and both are why the method and criteria above were amended:

1. **Leakage.** Join depth was derived from `reference_sql` — the gold answer. The result
   establishes that join depth *separates the failure class*, not that an *estimable* predictor
   does.
2. **Collinearity.** Family 3 is the only depth-3 family and is exactly 6 of the 30 questions, so
   `depth >= 3` and `family == 3` are the same predictor here. The effective sample size for the
   signal is **5 families, not 30 trials** — it recognizes one template family rather than
   generalizing.

Also worth recording: the one missed silent wrong sits at depth 1 — a failure with no complexity
signal at all. Even a perfect complexity router leaves that class untouched, which bounds the
ceiling of the whole pre-flight axis.

**Confounds to state in the result, whatever it says.** N=30, one seeded draw, one schema, five
template families, SQLite rather than Teradata dialect. A predictor tuned on the same 30 trials
it is scored against is fitted, not validated — so the honest output of Phase 0 is a
*go/no-go plus an effect size*, never an accuracy claim. A positive result earns a fresh
question set, not a feature.

**Cost:** no LLM calls, no dollars, no Ollama, no new dependencies. Runnable on any machine that
has the repo.

### Companion experiment — can EXPLAIN see the silent failures?

**S**'s central claim deserves the same treatment as **X**'s, and it is nearly as cheap.
The claim is narrow and testable: *a plan-shape check would have flagged the wrong joins
that the execution check could not see.*

**Method.** Regenerate the Phase 5 synthetic schema (the generator in
`leveling_sql_gen_test.go` is seed-pinned and parity-locked, so the database is
reproducible), then run `EXPLAIN QUERY PLAN` over the generated SQL already recorded in
`docs/experiments/sql_arms.jsonl` for the ladder arm. Score the plan-shape flag — a
cartesian/product join, or a scan of a table the reference plan does not scan — against the
recorded `silent_wrong` label. No model calls.

**Rejection criteria, fixed before running.** The EXPLAIN rung is **rejected** unless the
plan-shape flag catches at least **5 of the 7** silent-wrong queries while flagging no more
than **1 in 3** of the 23 others. A flag that fires on most correct queries is not a signal,
it is a tax.

⚠️ Two limits to state with the result whatever it says. SQLite's `EXPLAIN QUERY PLAN` is
far coarser than a Teradata plan — no cost estimates, no confidence levels, so this tests
the *weakest possible* version of rung 3 and a negative result would not condemn the real
one. And the wrong-arithmetic failures are invisible to any plan check by construction;
only the wrong-join class is in scope.

## Deferred decisions

Each one is deliberately open, and each is cheap to keep open under the invariants above.

1. Whether effort escalation precedes model escalation. Requires the 2×2 — fix model vary
   effort, fix effort vary model. **Note the conflict this creates:** ordering by information
   gain and ordering by catalog cost now disagree, and which wins is the interesting experiment
   rather than a design choice to make on paper. ⚠️ **This decision currently has no supporting
   data on either side** — every measured arm was free and local, so the record contains no
   cost-ordering evidence at all, and no local → frontier escalation to derive it from. See
   [What the record does not measure](#what-the-record-does-not-measure).
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
8. How deep the **default** check ladder should go. Rungs 1–3 are nearly free, but a default
   that executes or verifies changes the cost profile of every stage that opts in.
9. Whether a check that needs a tool call competes with the agent's tool budget, or gets its
   own. Invariant 10 says its *spend* is attributed to **S**; where the call is *counted*
   against a cap is still open.
10. The preset names, and who owns them. `Draft` / `Contract` / `Audit` are placeholders.
11. Whether `V` becomes a typed enum on the failure or stays free text. Keying **C** on it
    cheaply probably requires the enum.

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

No Go tests were run and none are claimed; no model was called. One replay analysis was
executed — `python3 docs/experiments/probe_join_depth.py`, which reads only committed data and
prints every figure quoted in this document's
[What the record does not measure](#what-the-record-does-not-measure) and
[Phase 0](#phase-0--the-zero-cost-replay) sections. What was verified in the v1.4.0 tree at
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
- ✅ **Executed.** Phase 5's per-arm figures reproduce exactly from the raw rows: 17/5/8,
  23/7/0, 23/7/0 and 30/0/0 correct / silent-wrong / exec-error; 30/38/38/30 calls; 0/0/0/30
  strong-model calls; 25.3/30.4/29.7/475.2 seconds. Escalations on the seven silent-wrong
  trials were `[0, 0, 0, 0, 0, 1, 1]`.
- ✅ **Executed.** Every attempt in all four arms ran one of two local Ollama models
  (`llama3.2:latest`, `deepseek-r1:latest`), and rung 1 in both leveled arms was
  `llama3.2:latest` — so no escalation crossed a tier boundary. No cost field exists in the
  rows.
- ⚠️ **Executed, and it amended this plan.** The Phase 0 predictor pre-check passes the
  originally-written bar (6 of 7 recovered, 0 of 23 false flags) but is leaky — derived from
  `reference_sql` — and collinear with template family, so its effective N is 5. The Phase 0
  method and rejection criteria were rewritten in response.
- ✅ The check ladder's cheap rungs have primitives: `fabric.GetSchema` returns field name,
  type, nullability and primary key (`pkg/fabric/interface.go:122-142`); `ExecuteQuery` can
  carry an `EXPLAIN`; `Capabilities.Features` / `SupportedOperations`
  (`pkg/fabric/interface.go:163-184`) is where a backend would advertise EXPLAIN or DBQL
  reachability.
- ⚠️ No EXPLAIN **plan parser** exists. `pkg/mcp/apps/html/explain-plan-visualizer.html` is
  registered as an MCP UI resource (`pkg/mcp/apps/embedded.go:32,78`) with nothing in Go
  producing plan data for it — a renderer, not a parser.
- ⚠️ DBQL appears only as domain knowledge inside Teradata performance patterns
  (`patterns/teradata/performance/`), for *analysing* a system. Nothing uses it to verify a
  generated query.
- ⚠️ No `leveling:` block appears in any shipped example workflow;
  `examples/reference/workflows/workflow-all-fields-reference.yaml` does not mention it. Worth
  fixing independently of this plan.
- 📋 Every *measurement* cited in this document comes from
  [plan-capability-leveling.md](plan-capability-leveling.md) (Phases 2b–5). No new experiment
  was run and no model was called; the three ✅/⚠️ **Executed** entries above are replay
  analysis over that record's committed data, reproducible with
  `python3 docs/experiments/probe_join_depth.py`.

## Rendering this record as a page

This Markdown is the source of record. A styled standalone HTML version is generated from it —
never hand-edited — by `scripts/render-plan-page.py`:

```bash
python3 scripts/render-plan-page.py docs/plan-effort-leveling.md \
  -o /tmp/effort-leveling-plan.html --title "Effort Leveling Plan"
```

The renderer is stdlib-only (the repo has no pandoc and no Markdown package) and supports the
subset these plan records use: headings, pipe tables, lists with hanging continuations,
blockquotes, GitHub alerts (`> [!WARNING]`), rules, and inline code/bold/italic/links. Four
source conventions get a visual treatment: the `**Key**: value` run under the H1 becomes the
masthead, a `## Table of Contents` list becomes the sticky side nav, ✅/⚠️/📋/❌/🚧 become status
pills, and a table cell holding a bare control key (`A`, `B1`, `A × B1`) is set in the accent
mono face. Relative links to repo files are de-linked to text plus path, because they would
404 once the page is published away from the repo. Anything outside the subset renders as
visible escaped text rather than being dropped.

`--title` exists so a published page keeps a stable name when the H1 is reworded. The generated
HTML is build output and is not committed.

## See Also

- [Capability Leveling design record](plan-capability-leveling.md) — the measured record this
  plan re-reads, including the C2 and C4 rejections
- [Workflow Capability Leveling Reference](reference/workflow-leveling.md) — the shipped model
  axis: tiers, ladder, bounds, failure semantics
- [Workflow Output Retry Reference](reference/workflow-output-retry.md) — control **C** as it
  exists today
