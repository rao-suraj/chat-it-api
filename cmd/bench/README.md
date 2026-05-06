# LLM Benchmark — Mode A (single-LLM with tool calling)

Tests whether a single Groq LLM call with tool calling can handle the appointment-booking domain
well enough to skip a separate intent classifier + DST.

## What it measures

| Metric | What it tells you |
|---|---|
| **Tool selection accuracy** | Did the model pick the right tool (or correctly stay silent)? This is the model's intent classification. |
| **Slot extraction accuracy** | When the right tool was called, did all the expected slot values match? |
| **Fully correct** | Tool + slots both right. The headline number. |
| **Latency p50 / p95 / p99** | What real users will feel. p99 is the slow tail you can't ignore. |
| **Cost per turn** | Drives unit economics. |

## How to run

```bash
# 1. Set your key
export GROQ_API_KEY=gsk_...

# 2. Default run (Llama 3.3 70B on the v1 dataset)
go run ./cmd/bench

# 3. Try a faster cheaper model
go run ./cmd/bench --model=llama-3.1-8b-instant

# 4. All flags
go run ./cmd/bench \
  --dataset=cmd/bench/dataset/appointment_v1.json \
  --model=llama-3.3-70b-versatile \
  --today=2026-04-30 \
  --concurrency=4 \ 
  --temperature=0 \
  --max_tokens=256 \
  --out=cmd/bench/results.json
```

> **Note on `--today`**: the dataset has hard-coded expected dates assuming today is 2026-04-30
> (Thursday). Keep `--today=2026-04-30` until you regenerate the dataset, otherwise relative-date
> cases like "tomorrow" and "Monday" will fail.

## Interpreting results

After a run you'll see something like:

```
=== Benchmark Report ===
Model:        llama-3.3-70b-versatile
Total cases:  36
Errors:       0

Accuracy:
  Tool selection:    34/36  (94.4%)
  Slot extraction:   28/30  (93.3%)
  Fully correct:     32/36  (88.9%)

Latency (ms):
  p50:   421
  p95:   978
  p99:   1124

Tokens & cost:
  Avg input:    612
  Avg output:   38
  Cost / turn:  $0.00039
```

### Decision rule

These are starting thresholds — adjust to your business reality.

| Fully correct | p95 latency | Decision |
|---|---|---|
| **≥ 90%** | **≤ 1.5s** | Ship Mode A. Skip the multi-tier architecture. |
| 75–90% | acceptable | Improve prompt + tools first. Re-run. Don't jump to multi-tier yet. |
| < 75% | any | Investigate failures. Could be prompt issues, tool schema, or genuinely a model-capability gap. |
| any | > 3s p95 | Try `llama-3.1-8b-instant` and re-run. Speed is usually a model choice, not an architecture problem. |

The trap to avoid: jumping to a multi-tier architecture because Mode A failed at *prompt engineering*.
Most "the LLM can't do it" failures are actually prompt or tool-schema problems.

## What's in the dataset

36 cases covering:
- Booking with full info, with contact info (→ create_appointment), with missing slots
- Reschedule (with ID, without ID, vague)
- Cancel (with various ID formats, without ID)
- Availability checks (full/partial info)
- Status checks
- List services, pricing, business info
- Greetings, farewells
- Out-of-scope (weather, jokes)

Failures break down into useful categories:
- **Wrong tool** → intent classification failure
- **No tool when one expected** → over-cautious; tighten the prompt
- **Tool when none expected** → over-eager; add explicit no-tool guidance
- **Slot value mismatch** → date/time normalization, service name normalization

The failures listed at the bottom of the report tell you exactly which bucket each one falls into.

## Cost estimate for one run

~36 cases × ~650 tokens avg = ~23k tokens. On Llama 3.3 70B that's roughly **$0.015 per full run**.
You can run this 100 times for $1.50 — iterate freely.

## Next steps

1. **Run it.** Get baseline numbers.
2. **Look at the failures.** Most will cluster into 2–3 categories.
3. **Iterate on the system prompt** in `prompts.go`. Re-run.
4. **Try the smaller model.** Often 8B is "good enough" at 1/10 the cost and 3x the speed.
5. **If Mode A is good enough** → forget the multi-tier plan, build out `chat-it-api` chat endpoint with this exact pattern.
6. **If Mode A is not good enough** → only THEN port the same dataset to a Mode B harness (intent classifier → DST → generator) and compare directly.

## Files

```
cmd/bench/
├── README.md
├── main.go            # entry, runner, reporting
├── groq.go            # Groq API client (OpenAI-compatible)
├── prompts.go         # system prompt + tool definitions
├── scorer.go          # accuracy scoring
└── dataset/
    └── appointment_v1.json
```

No new dependencies — uses `slog` and `github.com/pkg/errors` from your existing project.
