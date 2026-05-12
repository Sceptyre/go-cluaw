---
name: scientific-method
description: A 6-step scientific method workflow for systematic software engineering. Use when the task is complex, ambiguous, the problem is not well understood, or when previous attempts have failed. Provides discipline to avoid jumping to conclusions.
compatibility: Designed for coding agent harnesses (opencode, codex, and compatible products)
metadata:
  author: user
  version: "1.0.0"
  steps: "observe,hypothesize,experiment,analyze,conclude,feedback"
---

# Scientific Method

Follow these six phases in order. Do not skip or reorder them. Each phase has a clear exit condition — do not proceed until it is satisfied.

## When to Use This Skill

- The task involves more than one file or concept
- The root cause is unknown (debugging, investigation)
- Multiple approaches are possible and the best one is unclear
- A previous attempt failed and you need a fresh approach
- The requirements are ambiguous or incomplete

## Phase 1: Observe

Gather comprehensive context before proposing any solution.

### Instructions

1. **Read the primary file(s)** referenced in the request. Understand their structure, purpose, and interfaces.
2. **Search related files** — tests, imports, callers, configuration. Understanding the system matters more than the single file.
3. **Check error messages and logs** if debugging. Get the exact text, stack trace, and any reproduction steps.
4. **Search existing memory or past sessions** for relevant context on this codebase, previous attempts, or known patterns.
5. **Document unknowns** explicitly. List what you do not yet know.

### Exit Condition

You can summarize the current state in 3-5 sentences covering: what the code does, what is expected, and what is actually happening. If you cannot, you have not observed enough.

## Phase 2: Hypothesize

Form an explicit, testable hypothesis before writing code.

### Instructions

1. **State the hypothesis** using a causal format:
   - "I believe that if I change `X`, then `Y` will happen, because `Z`."
   - "I believe the root cause is `X`, because the error appears when `Y` and not when `Z`."
2. **Define 2-3 specific success criteria.** These must be falsifiable — you must be able to clearly determine whether each was met.
   - Good: "The test suite passes," "The response time drops below 200ms," "The error message no longer appears"
   - Poor: "The code is cleaner," "Performance improves"
3. **List the tools and commands** you expect to use and in what order.
4. **Rate your confidence** (low / medium / high) and state why. Low confidence means the experiment should be the smallest possible.
5. **Check for alternative hypotheses.** If you have only one hypothesis, pause and generate at least one alternative.

### Exit Condition

You have written (in your working memory) a clear hypothesis statement, 2-3 success criteria, and a planned tool sequence. You can articulate what result would falsify your hypothesis.

## Phase 3: Experiment

Execute the minimal change needed to test the hypothesis.

### Instructions

1. **Prefer the smallest possible experiment.** A single file change, a single command, a single variable change. If the experiment involves multiple changes, split it into sequenced mini-experiments.
2. **Collect all output.** Save command output, error messages, and timing data. Do not discard anything yet.
3. **Run validation.** Tests, linters, type checkers, or whatever verification exists. Compare output to a baseline if possible.
4. **If the experiment fails (error, crash):** Still collect the output. A failed experiment is still data.
5. **Do not change the hypothesis mid-experiment.** If new information suggests a different approach, complete the current experiment first, then start a new cycle from Phase 1.

### Exit Condition

You have executed the planned experiment and collected the results. You have the data needed to evaluate each success criterion.

## Phase 4: Analyze

Compare actual results against predictions.

### Instructions

1. **Evaluate each success criterion** from Phase 2. Mark each as met or not met.
2. **Compare actual output to each prediction.** Note exactly where they diverge.
3. **Document unexpected results.** Anything surprising is a signal — the system does not behave as you assumed.
4. **Assess overall outcome:**
   - All criteria met → hypothesis confirmed
   - Some criteria met → hypothesis partially confirmed, needs refinement
   - No criteria met → hypothesis refuted

### Exit Condition

You can state: "The hypothesis was [confirmed / partially confirmed / refuted]. The evidence shows [specific finding]."

## Phase 5: Conclude

Decide what to do based on the analysis.

### Instructions

1. **If hypothesis confirmed:** Proceed with the change. Clean up any temporary diagnostics. Run the full test suite. Commit or apply the change.
2. **If hypothesis partially confirmed:** Identify what needs refinement. Either tighten the hypothesis and re-enter at Phase 3, or broaden the observation and re-enter at Phase 1.
3. **If hypothesis refuted:** Roll back all experimental changes. Re-enter at Phase 1 with the new information you gathered.
4. **Synthesize a summary.** Write 2-3 sentences covering: what was tried, what happened, what was learned.

### Exit Condition

You have either (a) committed a confirmed change, or (b) rolled back and documented what was learned for the next cycle.

## Phase 6: Feedback

Record learnings to improve future performance.

### Instructions

1. **Document what worked and what did not.** Be specific enough that a future session (or a different agent) can benefit.
2. **If this is a recurring pattern** (e.g., a common build issue, a tricky API behavior), consider creating or updating a skill to encode the solution.
3. **If the hypothesis was refuted**, record the false assumption that led to it. This prevents repeating the same mistake.
4. **Summarize the cycle duration.** How many iterations did it take? What would have made it faster?

### Exit Condition

You have written a feedback record covering: hypothesis, result, learnings, and applicable patterns for future tasks.

## Common Pitfalls

| Pitfall | Recovery |
|---|---|
| Skipping Observation and jumping straight to code | Roll back. Read the file. Search the codebase. Do not write code yet. |
| Hypothesis too vague to falsify | Rewrite with specific, measurable criteria. If you can't define criteria, you don't understand the problem well enough. |
| Experiment too large | Split into independent sub-experiments. Smaller experiments produce sharper evidence. |
| Ignoring unexpected results | Unexpected results are the most valuable data. Stop and investigate before proceeding. |
| Changing hypothesis mid-experiment | Complete the experiment, then start a new cycle. Mixing hypotheses confuses the evidence. |

## Integration Notes

- This skill works alongside domain-specific skills. Use it as the orchestration layer; let other skills provide specialized instructions for individual phases (e.g., debugging patterns, refactoring patterns).
- The phases map to common agent loop structures. Some harnesses may automate parts of this cycle — adapt the instructions to fit the agent's capabilities without abandoning the discipline.
