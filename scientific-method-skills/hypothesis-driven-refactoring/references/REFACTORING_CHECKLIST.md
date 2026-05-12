# Refactoring Checklist

A printable checklist for each phase of hypothesis-driven refactoring. Use this alongside the main SKILL.md.

---

## Phase 1: Baseline

- [ ] Read the file(s) to be changed end-to-end
- [ ] Read existing tests for the module
- [ ] Identify all callers and their expectations
- [ ] Run the test suite — confirm it is green
- [ ] Measure current metrics (complexity, performance, coverage — whatever is relevant)
- [ ] Note any known pain points or prior comments about this code
- [ ] Document the interface contract (what must not change)

## Phase 2: Predict

- [ ] State hypothesis using "If X → Y because Z" format
- [ ] Define 2-3 specific, measurable success criteria
- [ ] Define the rollback trigger (what result would cause a revert?)
- [ ] Identify the smallest meaningful validation change
- [ ] List the assumptions that, if wrong, would invalidate the hypothesis
- [ ] Rate confidence (low / medium / high) and state why

## Phase 3: Validate

- [ ] Implement ONLY the minimal validation change
- [ ] Run the specific reproduction / test for the changed area
- [ ] Rerun the same metrics from Phase 1
- [ ] Record before-and-after values
- [ ] If anything breaks: revert immediately (do not fix in this pass)

## Phase 4: Evaluate

- [ ] Check each success criterion against measured data
- [ ] Check for unexpected side effects (new warnings, slower build, etc.)
- [ ] Determine: confirmed, inconclusive, or refuted?
- [ ] Make a clear decision: integrate, iterate, or roll back

## Phase 5: Integrate

- [ ] Complete the full refactoring (extend from minimal to full coverage)
- [ ] Run the full test suite (not just the changed module)
- [ ] Remove temporary instrumentation (extra logs, benchmarks, debug flags)
- [ ] Update documentation if APIs or configuration changed
- [ ] Commit with hypothesis-and-evidence message format

## Phase 6: Codify

- [ ] Document outcome (what was tried, what happened, what was learned)
- [ ] Update linter rules or style guides if new patterns were established
- [ ] If refuted: record the wrong assumption
- [ ] Consider extracting a reusable skill for this type of refactoring

---

## Common Refactoring Types and Risks

### Extract Function / Method

**Risk**: Over-extraction creates fragmentation. A function that is only called once and has no clear identity should not be extracted.

**Validation**: After extraction, the caller should be strictly simpler. If it is not, revert.

### Rename

**Risk**: Missed references. Renaming a public symbol breaks callers outside the changed file.

**Validation**: Build must pass without errors. Search for all references before and after.

### Change Data Structure

**Risk**: Performance regression. Switching from array to map, or from struct to interface, changes memory layout and access patterns.

**Validation**: Run benchmarks. Do not rely on intuition.

### Introduce Interface / Abstraction

**Risk**: Leaky abstraction. The interface may need to change as new implementations are added.

**Validation**: There should be at least two concrete implementations planned before introducing an interface. If there is only one use case, interfaces add complexity without benefit.

### Split Module

**Risk**: Circular dependencies. The split may create a dependency cycle between the new modules.

**Validation**: Check that no import cycle exists. Run the dependency analyzer.

### Inline / Merge

**Risk**: Loss of modularity. Inlining removes abstraction boundaries.

**Validation**: The merged code should not exceed a reasonable complexity threshold for the language and project conventions.

### Change Error Handling Strategy

**Risk**: Silent failures. Changing from panic/throw to error return (or vice versa) may cause callers to overlook failures that were previously fatal.

**Validation**: Audit all callers after the change. Each error return must be handled or explicitly ignored.

### Dependency Upgrade

**Risk**: Breaking API changes. The new version may have removed or altered functions the code depends on.

**Validation**: Run the full test suite. Check the changelog for every removed API. Search for any use of deprecated symbols in the new version.

---

## Commit Message Template

When committing a confirmed refactoring:

```
<type>: <short description>

## Hypothesis
<restate the hypothesis>

## Evidence
| Criterion | Before | After | Status |
|-----------|--------|-------|--------|
| <criterion 1> | <value> | <value> | met / not met |
| <criterion 2> | <value> | <value> | met / not met |

## Learnings
<unexpected findings, insights, or notes for future>
```
