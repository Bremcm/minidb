# minidb

A SQL database engine written from scratch in Go — lexer, parser, executor, page-based storage, B+tree index, and write-ahead logging. No database libraries, no ORM, no `database/sql`. Bytes on disk are laid out by hand.

```sql
> CREATE TABLE users (id INT, name TEXT, age INT)
  table "users" created
> INSERT INTO users VALUES (5, 'bob', 30)
> INSERT INTO users VALUES (2, 'alice', 25)
> SELECT name, age FROM users WHERE age > 26 AND name = 'bob'
  name  age
  ----  ---
  bob   30
  (1 row)
```

## Two things worth knowing up front

**Index lookup is ~127,000× faster than a full scan.** Measured on 1M rows, Apple M4:

| Operation | Time | Allocations |
|---|---|---|
| B+tree point lookup | 400 ns | 1 |
| Full scan to the same key | 51 ms | ~1,000,000 |

The index touches ~4 pages (`log n` descent). The scan walks every leaf and deserializes every row.

**It survives `kill -9`.** Not a simulated failure — an actual process killed by `SIGKILL` mid-write, twenty times in a row:

```
=== crash recovery test: 20 rounds ===
round 1: ok (killed after 0.210s)  → 1000 rows, keys 1..1000 intact
round 2: ok (killed after 0.210s)  → 2800 rows, keys 1..2800 intact
...
round 20: ok (killed after 0.438s) → 49700 rows, keys 1..49700 intact
=== all rounds passed ===
```

After each kill the database reopens, replays its log, and verifies that every key is present, ordered, and reachable by both sequential scan and index descent. Disable the WAL and the tree corrupts within a couple of rounds.

## Architecture

```
SQL text
   │
   ▼  lexer/        two-pointer scan, lookahead for >= != <>
tokens
   │
   ▼  parser/       recursive descent + Pratt expression parsing
AST                 (comparison > AND > OR, parens override)
   │
   ▼  engine/       type checking, WHERE evaluation, projection
   │                recognizes "key = const" and takes the index path
   ▼  storage/      Value/Row serialization, catalog, B+tree
   │
   ▼  storage/disk/ 4KB pages, slotted layout, pager with dirty tracking
   │
   ▼  storage/wal/  append-only log, CRC32, crash recovery
disk
```

Each layer only knows about the one below it. Storage was rewritten twice — flat slice → paged file → B+tree — and the lexer, parser, and evaluator were never touched. Stage-3 tests passed unmodified through both rewrites.

## How rows sit inside a page

Variable-length rows in a fixed 4KB page, with no shifting when one row changes size:

```
byte 0                                              byte 4096
  ├──────────┬──────────────┬─────────────┬─────────────────┤
  │ HEADER   │  SLOT DIR    │    FREE     │      DATA       │
  │ 8 bytes  │  grows →     │             │    ← grows      │
  └──────────┴──────────────┴─────────────┴─────────────────┘
             slot0 slot1 …               … row1  row0
```

Slots are fixed-width (offset + length), so row *i* is one jump away. Row payloads live wherever they fit; their order in memory has nothing to do with their order in the directory. The two regions grow toward each other, and free space is the single gap between them.

B+tree nodes use a variant: keys form a dense `int64` array at a fixed offset so binary search can index directly, with data slots (leaves) or child page IDs (internal nodes) after them.

## Durability protocol

```
1. BEGIN               → log
2. page images         → log
3. COMMIT + fsync      → log        ← point of no return
4. write pages         → data file
5. fsync               → data file
6. truncate log                     ← checkpoint
```

Crash before step 3: the transaction was never committed, its pages are ignored, data is untouched.
Crash after step 3: recovery replays the logged page images and finishes the job.
Crash during recovery: replay is idempotent (full page images), so it just runs again.

## Try it

```bash
go build ./cmd/minidb && ./minidb mydb.db
```

```bash
# benchmarks
go test ./internal/storage/ -bench . -benchmem -run '^$' -benchtime 10x

# crash recovery (builds, kills, verifies, 20 rounds)
./scripts/crashtest.sh
```

## Supported SQL

```sql
CREATE TABLE t (id INT, name TEXT)
INSERT INTO t VALUES (1, 'alice')
SELECT * FROM t
SELECT name FROM t WHERE id = 1
SELECT name FROM t WHERE (age > 30 OR name = 'bob') AND age = 30
```

Operators: `= != <> < > <= >=`, `AND`, `OR`, parentheses. Types: `INT`, `TEXT`, `NULL`.

Comparing mismatched types is an error rather than a silent coercion — a database shouldn't guess. `NULL` propagates through comparisons instead of collapsing to false, which is why `WHERE x = NULL` never matches anything, exactly as in real SQL.

## What's honest, what's simplified

Deliberately scoped down, and worth stating plainly:

- **Index key is the first `INT` column.** No explicit `PRIMARY KEY`, no composite or `TEXT` keys. The tree mechanics don't change; the key encoding would.
- **No `DELETE` or `UPDATE`.** Deletion in a B+tree requires merging and redistributing underfull nodes — a separate problem from insertion, and a bigger one.
- **Single-threaded.** No locking, no MVCC, no concurrent readers.
- **The page cache is unbounded.** Every page ever read stays resident. Real systems evict by LRU; that machinery is missing, not hidden.
- **No SQL-level transactions.** WAL protects against crashes; `BEGIN`/`ROLLBACK` as user-facing statements aren't implemented.
- **Physical logging.** Full 4KB page images in the log — simple and idempotent, but the log is fat compared to logical or physiological logging.

## Two bugs worth remembering

**Split left dead space behind.** When a full leaf split, the right half was copied out and the left half was "truncated" by lowering the key count — but the free-space boundary was never recomputed. The moved rows still counted as occupied. The next insert wrote into that space and overwrote live data.

Short payloads hid it completely; there was so much slack that the overlap hit nothing. Long payloads exposed it as scans jumping backward.

This is the same problem Postgres runs an entire background process for. Understanding *why* `VACUUM` exists is different from reading that it does.

**The first benchmark lied.** It reported a full scan of a million rows in 783 nanoseconds — physically impossible, and plausible-looking enough to almost accept. The scan searched for keys `0..9`, found them in the first leaf, and stopped immediately. It measured nothing.

Searching keys near the *end* of the tree turned 783 ns into 51 ms and revealed the real gap. Benchmarks fail quietly: they always produce a number.

## Layout

```
cmd/minidb        REPL
cmd/crashtest     insert loop + integrity checker for the kill test
internal/lexer    SQL → tokens
internal/parser   tokens → AST
internal/ast      node definitions (sealed interfaces)
internal/engine   execution, type checks, index-vs-scan decision
internal/storage  Value/Row encoding, catalog, B+tree
  └── disk        pages, slotted layout, pager
  └── wal         write-ahead log, recovery
scripts           crash test harness
```

Test coverage sits where correctness is easy to get subtly wrong: operator precedence changing result sets, insertion from shuffled input staying ordered across thousands of splits, serialization round-trips at `int64` boundaries, truncated and corrupted log records, and full persistence across reopening.
