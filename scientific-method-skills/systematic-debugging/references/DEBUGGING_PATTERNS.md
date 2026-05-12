# Debugging Patterns by Symptom Type

A quick-reference catalog of common failure patterns, organized by what you observe. Use when forming hypotheses in Step 2 (Isolate).

---

## Compilation / Type Errors

| Symptom | Common Root Causes | Fastest Diagnostic |
|---|---|---|
| "undefined symbol" / "cannot find name" | Missing import, typo in name, file not in build | Check imports, check file is included in build config |
| "type mismatch" | Wrong type passed, missing conversion, interface not satisfied | Trace the types backwards from the error site |
| "missing method" | Interface not fully implemented, pointer vs value receiver | Check the receiver type matches the interface |
| Circular dependency | Package A imports B, B imports A (directly or transitively) | `go mod graph` / dependency analyzer |
| "multiple definition" | Header included twice (C/C++), duplicate file in build | Check include guards, check build file list |

## Runtime Crashes

| Symptom | Common Root Causes | Fastest Diagnostic |
|---|---|---|
| Segmentation fault / null pointer | Dereferencing nil, use-after-free, buffer overflow | Add nil checks before the crash site, enable address sanitizer |
| Stack overflow | Infinite recursion, excessively deep call chain | Count the recursion depth, look for missing base case |
| Out of memory | Memory leak, unbounded data structure, too-large allocation | Heap profile, limit test input size |
| Panic / unhandled exception | Missing error check, unexpected nil, index out of bounds | Trace the stack: which call produced the invalid state? |
| Deadlock / hang | Lock ordering violation, missing unlock on error path, channel deadlock | `SIGQUIT` / `SIGABRT` to dump goroutine/thread stacks |

## Logic Bugs

| Symptom | Common Root Causes | Fastest Diagnostic |
|---|---|---|
| Wrong output | Off-by-one, wrong comparison operator, incorrect algorithm | Print intermediate values at each transformation step |
| Edge case not handled | Empty input, zero value, boundary condition, missing enum variant | Test with empty string, zero, max value, null |
| "It works on my machine" | Environment difference: locale, timezone, file encoding, dependency version | `diff` the environment variables, pinned dependency versions |
| Intermittent failure | Race condition, uninitialized variable, cache staleness | Add `-race` flag / thread sanitizer, run 10x to check determinism |
| Timing-dependent | Clock skew, timeout too short, async race | Add precise timing logs, increase timeout to rule it out |

## Test Failures

| Symptom | Common Root Causes | Fastest Diagnostic |
|---|---|---|
| Test passes in isolation but fails in suite | Shared state leak (global, static, env var), test ordering dependency | Run the failing test alone, then run it after each other test individually |
| Flaky test (passes sometimes) | Race condition, time-dependent assertion, random seed | Run 100x with `-count=100`, look for non-determinism |
| Golden file / snapshot mismatch | Output format changed, newline difference, platform-specific output | `diff` the actual vs expected output |
| Performance regression | Algorithmic change, lost optimization, increased allocation | Compare benchmark results before vs after, profile |

## Data / Persistent State

| Symptom | Common Root Causes | Fastest Diagnostic |
|---|---|---|
| Database query returns wrong results | Wrong join, missing filter, data type coercion | Log the actual SQL being executed, check parameter values |
| Migration fails | Schema mismatch, data incompatibility, partial migration | Run migration against a copy, check each step |
| Caching serves stale data | Cache invalidation missed, TTL too long, write-through not implemented | Add cache-key logging, check invalidation triggers |
| Serialization / deserialization fails | Field name mismatch, version skew, unsupported type | Print the raw bytes at the serialization boundary |

## Network / API

| Symptom | Common Root Causes | Fastest Diagnostic |
|---|---|---|
| Timeout | Network partition, server overload, firewall, DNS | `curl -v` / `wget` to check connectivity, check DNS resolution |
| 401 / 403 | Expired token, wrong credentials, missing permission | Check token expiry, check the exact URL being called |
| 500 / 502 / 503 | Server crash, upstream timeout, proxy misconfiguration | Check server logs, not just the HTTP response |
| Incorrect response body | Version mismatch in API contract, wrong endpoint, response transformation bug | Compare raw response vs documented schema |
| Rate limiting | Too many requests, missing backoff, no retry strategy | Check `Retry-After` header, add exponential backoff |

## Build / CI / Deployment

| Symptom | Common Root Causes | Fastest Diagnostic |
|---|---|---|
| "command not found" | Missing dependency in CI image, wrong PATH | Compare CI image to local environment |
| Build succeeds locally but fails in CI | Different OS/arch, different tool version, missing cache | Run the exact CI image locally |
| Deployment causes immediate rollback | Missing migration, environment variable not set, wrong artifact version | Compare the deployment diff to the previous working deployment |
| Test takes too long in CI | Integration tests running without isolation, shared resource contention | Profile the slowest tests, add `-short` mode |

## How to Use This Reference

When you encounter a symptom:

1. Find the symptom category that matches
2. Read the "Common Root Causes" and "Fastest Diagnostic" columns
3. Form a hypothesis based on the most likely cause
4. Run the recommended diagnostic
5. Interpret the result:
   - If the diagnostic confirms the hypothesis → proceed to fix
   - If it rules out the hypothesis → move to the next candidate
   - If it is inconclusive → refine the diagnostic or look for a different symptom category

This reference is not exhaustive. Update it when you encounter new patterns that are not covered.
