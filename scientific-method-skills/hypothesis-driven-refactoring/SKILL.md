---
name: hypothesis-driven-refactoring
description: A scientific-method approach to refactoring and code changes where outcomes are uncertain. Use when planning refactoring, performance optimization, dependency upgrades, or architectural changes. Ensures changes are validated before committing.
compatibility: Designed for coding agent harnesses (opencode, codex, and compatible products)
metadata:
  author: user
  version: "1.0.0"
---
# Hypothesis-Driven Refactoring

Refactoring is risky. You change working code to improve it, but "improve" is subjective and the change may introduce bugs. This skill applies the scientific method to reduce that risk: establish a baseline, predict the outcome, make the smallest possible change, measure the result, and decide based on evidence.

## When to Use This Skill

- Restructuring code to improve maintainability, readability, or modularity
- Performance optimization (speed, memory, network)
- Dependency upgrades (library, framework, toolchain)
- Architectural changes (moving from monolith to modules, changing patterns)
- Any change where "does this actually help?" is a real question

## Phase 1: Baseline (Observe)

Measure the current state before touching any code.

### Instructions

1. **Understand the contract.** What behavior must be preserved? Read the tests. Read the callers. The contract is whatever existing tests assert plus whatever callers depend on.
2. **Measure current metrics relevant to the change:**
   - **For performance work**: Benchmark results, p99 latency, memory profile, allocation count
   - **For maintainability work**: Cyclomatic complexity, file length, dependency count, test coverage
   - **For dependency upgrades**: Current version, changelog diff, known breaking changes
3. **Document the current interface signatures.** If the refactoring changes any public API, you need to know exactly what is changing.
4. **Run the existing test suite** and confirm it is green. You cannot measure improvement if the baseline is broken.
5. **Identify risk areas.** Which parts of the change are most likely to break? What has no test coverage? What is coupled to other systems?

### Exit Condition

You have a quantitative or clearly qualitative baseline for the aspect you intend to improve. You can articulate what must not change (the contract) and what you hope to change (the improvement target).

## Phase 2: Predict (Hypothesize)

State the expected improvement in falsifiable terms.

### Instructions

1. **Write the hypothesis using this template:**
   > "If I change `X` to `Y`, then `Z` will improve by `N%` / will not regress, because of `reason`."
2. **Define specific, measurable success criteria:**
   - Good: "The `handleRequest` function will have cyclomatic complexity <= 5 (down from 12)"
   - Good: "The `render` function will complete in < 50ms (currently 120ms)"
   - Good: "All existing tests pass without modification"
   - Poor: "The code will be cleaner" — not falsifiable
3. **Define the rollback trigger.** At what point will you revert and try a different approach? Decide this before you start coding.
4. **Identify the smallest meaningful change.** What is the minimum code change that can validate this hypothesis? Not the full refactoring — just enough to test the core prediction.

### Exit Condition

You have a written hypothesis with 2-3 success criteria, a rollback trigger, and a minimal validation plan. You know exactly what "done and validated" looks like.

## Phase 3: Validate (Experiment)

Implement the minimal change and measure the outcome.

### Instructions

1. **Make the minimum change** identified in Phase 2. Do not gold-plate. Do not refactor adjacent code. Do not rename unrelated variables.
2. **Run the tests** (or the specific reproduction case if this is a partial refactoring).
3. **Measure the outcome.** Rerun the same benchmarks, complexity analysis, or other metrics from Phase 1.
4. **Compare to baseline:**
   - Did the metric move in the expected direction?
   - Did anything unexpected change?
   - Are all existing tests still passing?
5. **If the experiment breaks something:** Stop. Revert. You gathered valuable data — the approach has a flaw. Do not try to fix it in the same pass. Start a new cycle.

### Exit Condition

You have before-and-after measurements for each success criterion. You know whether the hypothesis is supported or refuted by the evidence.

## Phase 4: Evaluate (Analyze)

Interpret the experimental results.

### Instructions

1. **Check each success criterion:**
   - All met → hypothesis confirmed. Proceed to full change.
   - Some met, some not → hypothesis partially correct. Identify which assumptions were wrong.
   - None met → hypothesis refuted. Roll back fully.
   - Metrics moved in the wrong direction → hypothesis refuted. Roll back.
2. **Check for hidden regressions.** Did test coverage drop? Did the change introduce new warnings? Did the build time increase? These are side effects not captured by the original criteria.
3. **Assess the magnitude.** Even if all criteria are met, was the improvement worth the change? A 1% speed gain for a 50% complexity increase may not be worth it.
4. **Decide:**
   - Confirmed → proceed to Phase 5 (Integrate)
   - Inconclusive → return to Phase 2 with refined hypothesis
   - Refuted → roll back and go to Phase 6 (Codify)

### Exit Condition

You have a clear decision: integrate, iterate, or roll back. The decision is based on measured evidence, not intuition.

## Phase 5: Integrate (Conclude)

Apply the confirmed change fully and finalize.

### Instructions

1. **Complete the full refactoring.** Extend the minimal change to cover all affected areas. The minimal experiment proved the approach; now apply it consistently.
2. **Run the full test suite.** Not just the relevant tests. A refactoring can have unexpected effects in distant modules.
3. **Update documentation** (if the change affects public API, configuration, or developer workflow).
4. **Remove temporary instrumentation** — any extra logging, benchmarking code, or debug flags added for the validation phase.
5. **Commit with a clear message** that references the hypothesis and the evidence that confirmed it.

### Exit Condition

The change is committed, all tests pass, and the commit message documents the hypothesis and evidence.

## Phase 6: Codify (Feedback)

Capture the learning for future refactoring decisions.

### Instructions

1. **Record the outcome:**
   - What was the hypothesis? (restate)
   - Was it confirmed or refuted?
   - What was the measured improvement?
   - What was unexpected?
2. **Update style guides or lint rules** if the refactoring established a new preferred pattern. If the team should always use the new approach, encode it in automation.
3. **If the hypothesis was refuted**, record the wrong assumption. This is valuable — knowing what does not work saves future time.
4. **Consider extracting a reusable pattern.** If this type of refactoring is common, create or update a skill with checklists for future similar changes.

### Exit Condition

The learning is documented and any applicable automation (linter rules, templates) has been updated.

## When Not to Use This Skill

| Situation | Why | Alternative |
|---|---|---|
| Bug fix | The priority is correcting behavior, not improving structure | Use `systematic-debugging` skill |
| Trivial rename / mechanical change | The outcome is certain | Just make the change |
| Security fix | The change is dictated by the vulnerability, not a hypothesis | Patch and ship |
| Experiment in a fork / branch with no commitment | The cost of being wrong is low | Use the branch as the experiment, no need for formal validation |

## References

See `references/REFACTORING_CHECKLIST.md` for a printable checklist and a catalog of common refactoring types with their associated risks.
