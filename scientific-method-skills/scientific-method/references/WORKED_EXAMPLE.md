# Worked Example: Debugging a Failing Test

This worked example walks through the full 6-phase scientific method for a realistic debugging scenario. All names and code are illustrative.

## Scenario

A developer reports: "The `SearchUsers` endpoint returns 500 when the query contains special characters. It worked yesterday."

## Phase 1: Observe

### What we do

1. **Read the handler file** (`handlers/users.go`). The `SearchUsers` handler calls `db.Query("SELECT * FROM users WHERE name ILIKE ?", "%"+q+"%")`.
2. **Read the test file** (`handlers/users_test.go`). Found test `TestSearchUsers_SpecialChars` that sends `q=o'brien`.
3. **Run the failing test** to get the exact error:
   ```
   pq: unterminated quoted string at or near "'"
   ```
4. **Search recent git history** for changes to `handlers/users.go`. Last change was 3 days ago — updated the query to add `ILIKE`.
5. **Check the query builder module** (`db/query.go`). Confirmed it uses `database/sql` parameterized queries, not string interpolation.

### What we know

- The handler passes a user-supplied string directly to a SQL `ILIKE` query via parameterized placeholder.
- The error `pq: unterminated quoted string` suggests PostgreSQL is receiving malformed SQL, which should not happen with parameterized queries.
- The error appeared after the ILIKE change, but parameterized queries should prevent injection regardless of operator.

## Phase 2: Hypothesize

### Hypothesis

"The ILIKE change introduced a separate string concatenation path that bypasses the parameterized query, because the developer concatenated the `%` wildcards directly into the query string instead of passing them as parameters."

### Success Criteria

1. [ ] Find the concatenation point in the code
2. [ ] Confirm the query sent to the database contains unescaped user input
3. [ ] Verify test passes after moving wildcards to the parameter value

### Tools Planned

1. `rg` / search for raw `%` concatenation in the handler or query builder
2. Read the git diff of the ILIKE commit
3. Enable query logging to capture the actual SQL sent

### Confidence

**Medium.** Parameterized queries are well understood, but the error string suggests raw SQL is being constructed somewhere. The ILIKE change is the most likely introduction point.

### Alternative Hypothesis

"The PostgreSQL driver has a bug with `ILIKE` and parameterized quotes." (Low probability, but worth ruling out.)

## Phase 3: Experiment

### Step 1: Search for concatenation

```bash
rg '%' handlers/users.go
```

Output: `db.Query("SELECT * FROM users WHERE name ILIKE ?", "%"+q+"%")`

Found it. The `%` wildcards are concatenated with `q` inside the argument list, but this is still a parameter — the driver should handle it. Let me verify.

### Step 2: Check the ILIKE commit diff

```bash
git log --all --oneline -- handlers/users.go | head -5
git show <hash> -- handlers/users.go
```

The diff shows the previous query used `LIKE ?` and now uses `ILIKE ?`. Same parameter passing. Nothing concatenated into the SQL string itself.

This contradicts the original hypothesis. The parameter passing looks correct.

### Step 3: Enable query logging (if available)

Added temporary logging before the query call:

```go
log.Printf("Query: %s, Args: %s", query, args)
```

Output: `Query: SELECT * FROM users WHERE name ILIKE ?, Args: [%o'brien%]`

The argument is being passed correctly as `%o'brien%`. The error must be happening elsewhere.

### Step 4: Search more broadly

```bash
rg -n 'o.brien\|special\|ILIK' handlers/ db/
```

Found in `db/query.go` — there is a custom query logging wrapper that does `fmt.Sprintf(query, args...)` for debug logging, *and uses the result for the actual query* instead of the parameterized version:

```go
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
    logged := fmt.Sprintf(query, args...)  // bug: interpolates for logging
    log.Printf("query: %s", logged)
    return db.conn.Query(logged)           // bug: passes interpolated string, not parameters
}
```

### Experimental Result

Found the root cause. The hypothesis was partially correct — there was a raw concatenation path, but it was in the wrapper layer, not in the handler. The wildcard placement was fine; the actual bug was passing the interpolated string to `db.conn.Query()` instead of passing query and args separately.

## Phase 4: Analyze

### Criteria Evaluation

1. [x] Find the concatenation point — Yes, in `db/query.go:12`
2. [x] Confirm the query sent to the database contains unescaped user input — Yes, logged SQL shows the raw string
3. [ ] Verify test passes after moving wildcards to the parameter value — Not yet, need to fix first

### Unexpected Result

The bug was not in the ILIKE change itself but in a pre-existing query wrapper that broke parameterization. The ILIKE change was a red herring — the error always existed for any special character in the LIKE pattern.

### Assessment

Hypothesis partially confirmed. The mechanism (raw concatenation) was correct but the location was wrong. Need to iterate with updated hypothesis.

## Phase 5: Conclude

### Decision

Do not roll back. The fix is clear: fix `db/query.go` to pass parameters correctly. Since the wrapper affects all queries, this fix has broader impact than just the reported bug.

### New Hypothesis

"Fixing the query wrapper to pass `args` separately to `db.conn.Query()` will resolve the 500 error for all special-character queries."

### Action

Edit `db/query.go`:

```go
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
    logged := fmt.Sprintf("%s %v", query, args)  // safe logging
    log.Printf("query: %s", logged)
    return db.conn.Query(query, args...)          // pass parameters correctly
}
```

Run the full test suite — all pass.

## Phase 6: Feedback

### What Worked

- Searching for the actual SQL string via logging pinpointed the problem immediately.
- Checking the git diff ruled out the surface-level hypothesis quickly.
- Broadening the search (`rg`) found the real culprit in the wrapper layer.

### What Could Be Faster

- Should have searched beyond the handler file in Phase 1. The `db/` package was directly relevant but not initially examined.
- Should have suspected the wrapper layer earlier — the error message `pq: unterminated quoted string` is a PostgreSQL parser error, which means raw SQL was reaching the server. A parameterized query would never produce this.

### Recorded for the Future

- **Pattern**: If a parameterized query produces a SQL parser error, there is a non-parameterized code path between the handler and the database driver. Search the data access layer, not the handler.
- **Tool**: Adding temporary query logging (print the actual SQL) is a fast way to distinguish parameter-vs-interpolation bugs.

### Skill Suggestion

Consider creating a `postgres-query-debugging` skill that codifies: how to enable query logging, how to distinguish driver-level vs application-level errors, and common wrapper-layer antipatterns.
