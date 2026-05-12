---
name: systematic-debugging
description: A disciplined approach to debugging using the scientific method. Use when encountering errors, crashes, test failures, or unexpected behavior. Prescribes a minimum-iteration path from symptom to verified root cause.
compatibility: Designed for coding agent harnesses (opencode, codex, and compatible products)
metadata:
  author: user
  version: "1.0.0"
---
# Systematic Debugging

Debugging is the scientific method applied to故障 (failure). This skill specializes the general scientific-method skill for troubleshooting. If you have not read the scientific-method skill, start there.

## When to Use This Skill

- An error message or stack trace is present
- A test is failing
- Behavior changed unexpectedly between two points in time
- A user reports behavior that does not match the specification
- Any "it was working before" scenario

## Step 1: Reproduce (Observe)

You must be able to reproduce the failure before you can fix it.

### Instructions

1. **Get the exact error.** Full text, stack trace, exit code, timestamp. Not "it crashes" — the actual output.
2. **Identify the reproduction path.** What input, sequence, or conditions trigger the failure? A single curl command, a test invocation, a specific sequence of UI events.
3. **Determine determinism.**
   - If the failure reproduces every time → deterministic. Good. You can bisect.
   - If intermittent → non-deterministic. Suspect races, timing, uninitialized memory, external state.
4. **Isolate the environment.** Is the failure specific to a platform, configuration, or data set? Try reproducing on a clean state.
5. **Check "what changed".** Search recent commits, dependency updates, environment changes, or configuration diffs.

### Exit Condition

You can reproduce the failure on demand and describe it in one sentence: "When I do `X`, the system produces `Y` instead of `Z`."

## Step 2: Isolate (Hypothesize)

Generate and prioritize candidate root causes.

### Instructions

1. **List candidate root causes.** Common categories in software:
   - **Logic error**: Off-by-one, wrong operator, missing edge case
   - **Data issue**: Null value, unexpected type, encoding problem, corrupted state
   - **Concurrency**: Race condition, deadlock, stale cache
   - **API / dependency change**: Upstream changed behavior, version mismatch
   - **Configuration**: Wrong flag, missing env var, incorrect path
   - **Environment**: OS difference, locale, permission, resource limit
2. **Prioritize by likelihood × ease of testing.** The best hypothesis to test first is the one most likely to be correct that is also fastest to check. A low-likelihood hypothesis that takes 2 seconds to test should be tested before a high-likelihood hypothesis that takes 10 minutes.
3. **Form specific, testable statements.** Not "there's a bug in the parser", but "if I pass this specific input to the parser, it returns an error because the regex does not escape the `+` character."
4. **Define the disconfirmation.** What result would prove this hypothesis wrong? If you cannot define it, the hypothesis is not testable.

### Exit Condition

You have 2-5 candidate root causes, each stated as a testable hypothesis with an ordering based on likelihood and test cost.

## Step 3: Diagnose (Experiment)

Execute diagnostic experiments to test each hypothesis.

### Instructions

1. **Test one hypothesis at a time.** Running multiple diagnostics simultaneously produces confounded results.
2. **Prefer the fastest test first.** A quick definitive test is better than a thorough ambiguous one.
   - Add a print / log statement (seconds)
   - Write a minimal reproduction script (minutes)
   - Use a binary search / bisect to narrow the change (minutes to hours)
3. **Three universal diagnostic techniques:**
   - **Add observability**: Insert logging at key points. What values flow through? What path is taken?
   - **Isolate variables**: Change one variable at a time. Does it still fail? Does it fail differently?
   - **Narrow the scope**: Remove parts until the failure disappears. The smallest reproducing input is the most informative.
4. **For non-deterministic bugs:** Run the experiment multiple times. If the failure appears 3/10 runs, a single success after the change is not evidence — you need statistical comparison.
5. **Collect all output.** Save logs, stack traces, timing data, and input values. You may need them to evaluate the next hypothesis if this one is ruled out.

### Exit Condition

You have run a diagnostic for the top hypothesis and collected the output. You can now evaluate whether the evidence supports or refutes it.

## Step 4: Evaluate (Analyze)

Assess the diagnostic evidence against each hypothesis.

### Instructions

1. **Rule out hypotheses systematically.** For each candidate:
   - If the diagnostic output matches the prediction → hypothesis survives (not confirmed yet, but still possible)
   - If the diagnostic output contradicts the prediction → hypothesis is refuted. Cross it off the list.
2. **Watch for confirmation bias.** The absence of evidence is not evidence of absence. If a diagnostic did not produce useful output, you did not test the hypothesis — you just ran a tool.
3. **Refine surviving hypotheses.** As you rule out candidates, the remaining ones become more likely. Update their priority.
4. **Check for compound causes.** Sometimes two things are both wrong and the failure only manifests when both coincide. If all single-variable diagnostics are inconclusive, consider two-variable experiments.

### Exit Condition

Either (a) you have identified the root cause with high confidence and can state "the failure is caused by `X`", or (b) you have ruled out all initial candidates and need to return to Step 1 with new observations.

## Step 5: Fix and Verify (Conclude)

Once the root cause is identified, apply a targeted fix and confirm it resolves the failure.

### Instructions

1. **Apply the minimal fix.** Exactly enough to address the root cause. Do not refactor unrelated code.
2. **Verify the failure is gone.** Run the exact reproduction case that failed before. It must pass.
3. **Verify no regressions.** Run the relevant test suite. A fix that breaks other things is not a fix.
4. **Consider systemic fixes.** If this bug represents a class of errors (e.g., all input validation is missing for this field type), fix the system, not just this instance.

### Exit Condition

The original reproduction case passes, and the test suite is green.

## Step 6: Prevent (Feedback)

Ensure the failure does not recur.

### Instructions

1. **Add a test** that would have caught this regression. If the bug escaped existing tests, the test gap is part of the root cause.
2. **Document the debugging path.** Write down: what the symptom was, what the root cause was, what diagnostic was most effective, and what assumption was wrong.
3. **Consider pattern encoding.** If this type of bug occurs frequently, create or update a skill to detect or prevent it.
4. **Review the process.** What would have found this bug faster? Better observability? A specific test? A linter rule?

### Exit Condition

A test is committed that covers this failure mode, and the debugging process is documented for future reference.

## Common Debugging Antipatterns

| Antipattern | Why It Fails | Better Approach |
|---|---|---|
| Changing code before understanding the error | You may fix the symptom, not the cause, and the fix may not generalize | Reproduce first, then read the error, then form a hypothesis |
| Only looking at the file mentioned in the stack trace | The bug may be in a caller, a dependency, or configuration | Search broadly — imports, callers, config, recent commits |
| Tweaking random things until it works | You learn nothing and may introduce subtle regressions | Form a hypothesis, test it, then fix |
| Assuming the bug is in the framework / library | This is almost never true early in debugging | Rule out application code first. If you cannot reproduce without the library, then investigate it. |
| Running multiple diagnostics in parallel | Results become confounded — you do not know which change caused the outcome | One experiment at a time |

## References

See `references/DEBUGGING_PATTERNS.md` for a catalog of common failure patterns organized by symptom type.
