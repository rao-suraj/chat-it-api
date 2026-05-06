package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/pkg/errors"
)

// --- Test case + result types ---

type Dataset struct {
	Metadata map[string]interface{} `json:"metadata"`
	Cases    []TestCase             `json:"cases"`
}

type TestCase struct {
	ID       string         `json:"id"`
	Input    string         `json:"input"`
	Expected ExpectedOutput `json:"expected"`
}

type ExpectedOutput struct {
	Intent   string         `json:"intent"`
	ToolCall *ToolCallSpec  `json:"tool_call"` // nil = no tool expected
	Notes    string         `json:"notes,omitempty"`
}

type ToolCallSpec struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type ActualOutput struct {
	AssistantMessage string        `json:"assistant_message,omitempty"`
	ToolCall         *ToolCallSpec `json:"tool_call,omitempty"`
}

type RunResult struct {
	TestCaseID   string         `json:"test_case_id"`
	Input        string         `json:"input"`
	Expected     ExpectedOutput `json:"expected"`
	Actual       ActualOutput   `json:"actual"`
	LatencyMs    int64          `json:"latency_ms"`
	InputTokens  int            `json:"input_tokens"`
	OutputTokens int            `json:"output_tokens"`
	IntentMatch  bool           `json:"intent_match"`
	ToolMatch    bool           `json:"tool_match"`
	SlotsMatch   bool           `json:"slots_match"`
	Error        string         `json:"error,omitempty"`
}

// --- Pricing (per million tokens) ---
//
// IMPORTANT: verify current Groq pricing at https://console.groq.com/docs/models
// and https://groq.com/pricing — these are placeholder approximations as of late 2024.
var groqPricing = map[string][2]float64{
	"llama-3.3-70b-versatile":  {0.59, 0.79},
	"llama-3.1-70b-versatile":  {0.59, 0.79},
	"llama-3.1-8b-instant":     {0.05, 0.08},
	"llama3-70b-8192":          {0.59, 0.79},
	"llama3-8b-8192":           {0.05, 0.08},
	"mixtral-8x7b-32768":       {0.24, 0.24},
	"gemma2-9b-it":             {0.20, 0.20},
}

func main() {
	var (
		datasetPath = flag.String("dataset", "cmd/bench/dataset/appointment_v1.json", "path to test dataset JSON")
		model       = flag.String("model", "llama-3.3-70b-versatile", "Groq model name")
		today       = flag.String("today", "2026-04-30", "today's date (YYYY-MM-DD) injected into the system prompt")
		concurrency = flag.Int("concurrency", 1, "max concurrent API calls (keep at 1 for free-tier TPM limits)")
		temperature = flag.Float64("temperature", 0.0, "sampling temperature (0 = deterministic)")
		maxTokens   = flag.Int("max_tokens", 256, "max output tokens")
		outPath     = flag.String("out", "cmd/bench/results.json", "path to write per-case results JSON")
		timeout     = flag.Duration("timeout", 30*time.Second, "per-request timeout")
		retries     = flag.Int("retries", 3, "max retries on 429 rate-limit responses")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "dev"
	}
	if err := godotenv.Load(".env." + appEnv); err != nil {
		slog.Debug("no env file loaded", "file", ".env."+appEnv)
	}

	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		fail("GROQ_API_KEY not set (set it in .env.%s or as an environment variable)", appEnv)
	}

	ds, err := loadDataset(*datasetPath)
	if err != nil {
		fail("load dataset: %v", err)
	}
	slog.Info("dataset loaded", "path", *datasetPath, "cases", len(ds.Cases), "model", *model, "today", *today)

	client := NewGroqClient(apiKey, *model, *timeout, *retries)
	tools := buildTools()
	sysPrompt := systemPrompt(*today)

	results := runAll(client, ds.Cases, sysPrompt, tools, *temperature, *maxTokens, *concurrency)

	// Persist per-case results.
	if err := writeResults(*outPath, results); err != nil {
		slog.Error("write results", "err", err)
	} else {
		slog.Info("results written", "path", *outPath)
	}

	printReport(results, *model)
}

func loadDataset(path string) (*Dataset, error) {
	abs, _ := filepath.Abs(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", abs)
	}
	var ds Dataset
	if err := json.Unmarshal(data, &ds); err != nil {
		return nil, errors.Wrap(err, "parse dataset")
	}
	if len(ds.Cases) == 0 {
		return nil, errors.New("dataset has no cases")
	}
	return &ds, nil
}

func runAll(client *GroqClient, cases []TestCase, sysPrompt string, tools []Tool, temp float64, maxTok, concurrency int) []RunResult {
	results := make([]RunResult, len(cases))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, tc := range cases {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, tc TestCase) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = runOne(client, tc, sysPrompt, tools, temp, maxTok)
			slog.Info("case done",
				"id", tc.ID,
				"latency_ms", results[i].LatencyMs,
				"tool_ok", results[i].ToolMatch,
				"slots_ok", results[i].SlotsMatch,
				"err", results[i].Error,
			)
		}(i, tc)
	}
	wg.Wait()
	return results
}

func runOne(client *GroqClient, tc TestCase, sysPrompt string, tools []Tool, temp float64, maxTok int) RunResult {
	// Allow enough time for all retries: each attempt gets the full HTTP timeout
	// plus up to ~10s of rate-limit wait, across maxRetries+1 total attempts.
	budget := client.timeout*time.Duration(client.maxRetries+1) + 10*time.Second*time.Duration(client.maxRetries)
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: tc.Input},
		},
		Tools:       tools,
		ToolChoice:  "auto",
		Temperature: temp,
		MaxTokens:   maxTok,
	}

	resp, latency, _, err := client.Chat(ctx, req)
	res := RunResult{
		TestCaseID: tc.ID,
		Input:      tc.Input,
		Expected:   tc.Expected,
		LatencyMs:  latency.Milliseconds(),
	}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if len(resp.Choices) == 0 {
		res.Error = "no choices in response"
		return res
	}

	msg := resp.Choices[0].Message
	res.Actual.AssistantMessage = msg.Content
	res.InputTokens = resp.Usage.PromptTokens
	res.OutputTokens = resp.Usage.CompletionTokens

	if len(msg.ToolCalls) > 0 {
		// Use the first tool call. Multi-tool turns are out of scope for this benchmark.
		tc0 := msg.ToolCalls[0]
		args, perr := ParseToolCallArgs(tc0.Function.Arguments)
		if perr != nil {
			res.Error = perr.Error()
			return res
		}
		res.Actual.ToolCall = &ToolCallSpec{Name: tc0.Function.Name, Args: args}
	}

	res.IntentMatch, res.ToolMatch, res.SlotsMatch = Score(tc.Expected, res.Actual)
	return res
}

func writeResults(path string, results []RunResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// --- Reporting ---

func printReport(results []RunResult, model string) {
	n := len(results)
	var (
		errs           int
		toolOK         int
		slotOKofTool   int
		slotEligible   int
		fullyCorrect   int
		totalIn        int
		totalOut       int
	)
	latencies := make([]int64, 0, n)
	for _, r := range results {
		if r.Error != "" {
			errs++
			continue
		}
		latencies = append(latencies, r.LatencyMs)
		totalIn += r.InputTokens
		totalOut += r.OutputTokens
		if r.ToolMatch {
			toolOK++
		}
		// Slot accuracy is only meaningful when a tool was expected & called correctly.
		if r.Expected.ToolCall != nil && r.ToolMatch {
			slotEligible++
			if r.SlotsMatch {
				slotOKofTool++
			}
		}
		if r.ToolMatch && r.SlotsMatch {
			fullyCorrect++
		}
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	pct := func(num, den int) float64 {
		if den == 0 {
			return 0
		}
		return float64(num) / float64(den) * 100
	}

	avgIn := 0
	avgOut := 0
	successful := n - errs
	if successful > 0 {
		avgIn = totalIn / successful
		avgOut = totalOut / successful
	}

	costPerTurn := estimateCostPerTurn(model, avgIn, avgOut)

	fmt.Println()
	fmt.Println("=== Benchmark Report ===")
	fmt.Printf("Model:        %s\n", model)
	fmt.Printf("Total cases:  %d\n", n)
	fmt.Printf("Errors:       %d\n", errs)
	fmt.Println()
	fmt.Println("Accuracy:")
	fmt.Printf("  Tool selection:    %d/%d  (%.1f%%)\n", toolOK, successful, pct(toolOK, successful))
	fmt.Printf("  Slot extraction:   %d/%d  (%.1f%%)  [of cases with correct tool + expected slots]\n", slotOKofTool, slotEligible, pct(slotOKofTool, slotEligible))
	fmt.Printf("  Fully correct:     %d/%d  (%.1f%%)\n", fullyCorrect, successful, pct(fullyCorrect, successful))
	fmt.Println()
	fmt.Println("Latency (ms):")
	fmt.Printf("  p50:   %d\n", percentile(latencies, 50))
	fmt.Printf("  p95:   %d\n", percentile(latencies, 95))
	fmt.Printf("  p99:   %d\n", percentile(latencies, 99))
	fmt.Printf("  max:   %d\n", percentile(latencies, 100))
	fmt.Println()
	fmt.Println("Tokens & cost:")
	fmt.Printf("  Avg input:    %d\n", avgIn)
	fmt.Printf("  Avg output:   %d\n", avgOut)
	if costPerTurn >= 0 {
		fmt.Printf("  Cost / turn:  $%.5f  (verify pricing for %s)\n", costPerTurn, model)
	} else {
		fmt.Printf("  Cost / turn:  pricing not defined for model %q — add it to groqPricing in main.go\n", model)
	}
	fmt.Println()

	// Failure listing.
	fmt.Println("Failures:")
	any := false
	for _, r := range results {
		if r.Error == "" && r.ToolMatch && r.SlotsMatch {
			continue
		}
		any = true
		fmt.Printf("  [%s] %q\n", r.TestCaseID, r.Input)
		if r.Error != "" {
			fmt.Printf("     ERROR: %s\n", r.Error)
			continue
		}
		fmt.Printf("     expected: %s\n", formatExpected(r.Expected))
		fmt.Printf("     actual:   %s\n", formatActual(r.Actual))
		if r.ToolMatch && !r.SlotsMatch && r.Expected.ToolCall != nil && r.Actual.ToolCall != nil {
			for _, d := range SlotDiff(r.Expected.ToolCall.Args, r.Actual.ToolCall.Args) {
				fmt.Printf("       - %s\n", d)
			}
		}
	}
	if !any {
		fmt.Println("  (none)")
	}
	fmt.Println()
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * (len(sorted) - 1)) / 100
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func estimateCostPerTurn(model string, inTok, outTok int) float64 {
	p, ok := groqPricing[model]
	if !ok {
		return -1
	}
	return (float64(inTok)/1_000_000)*p[0] + (float64(outTok)/1_000_000)*p[1]
}

func formatExpected(e ExpectedOutput) string {
	if e.ToolCall == nil {
		return fmt.Sprintf("intent=%s, no tool call", e.Intent)
	}
	args, _ := json.Marshal(e.ToolCall.Args)
	return fmt.Sprintf("intent=%s, tool=%s args=%s", e.Intent, e.ToolCall.Name, args)
}

func formatActual(a ActualOutput) string {
	if a.ToolCall == nil {
		msg := a.AssistantMessage
		if len(msg) > 80 {
			msg = msg[:80] + "..."
		}
		return fmt.Sprintf("no tool call, message=%q", msg)
	}
	args, _ := json.Marshal(a.ToolCall.Args)
	return fmt.Sprintf("tool=%s args=%s", a.ToolCall.Name, args)
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
