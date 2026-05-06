package main

import (
	"fmt"
	"regexp"
	"strings"
)

// toolToIntent maps a tool name to the intent it represents.
// This lets us derive intent from the model's tool selection without a separate classifier.
var toolToIntent = map[string]string{
	"check_availability":     "check_availability",
	"create_appointment":     "book_appointment",
	"cancel_appointment":     "cancel_appointment",
	"reschedule_appointment": "reschedule_appointment",
	"get_appointment_status": "check_status",
	"list_services":          "list_services",
	"get_pricing":            "ask_pricing",
	"get_business_info":      "ask_business_info",
}

// Score compares expected vs actual output and returns the three correctness flags.
//   - intentMatch: did the inferred intent match expected.intent?
//   - toolMatch:   was the right tool called (or correctly omitted)?
//   - slotsMatch:  if a tool was called, did all expected slot values match?
func Score(expected ExpectedOutput, actual ActualOutput) (intentMatch, toolMatch, slotsMatch bool) {
	expHasTool := expected.ToolCall != nil
	actHasTool := actual.ToolCall != nil

	switch {
	case !expHasTool && !actHasTool:
		// Both correctly chose no tool. Intent match is implicit (we trust expected.intent).
		return true, true, true

	case expHasTool && !actHasTool:
		return false, false, false

	case !expHasTool && actHasTool:
		return false, false, false

	default: // both have tool
		toolMatch = strings.EqualFold(expected.ToolCall.Name, actual.ToolCall.Name)
		if mapped, ok := toolToIntent[actual.ToolCall.Name]; ok {
			intentMatch = mapped == expected.Intent
		}
		if toolMatch {
			slotsMatch = compareSlots(expected.ToolCall.Args, actual.ToolCall.Args)
		}
	}
	return
}

// compareSlots checks every key in `expected` against `actual`.
// Extra keys in actual are tolerated. Missing or mismatched keys fail.
func compareSlots(expected, actual map[string]interface{}) bool {
	for k, exp := range expected {
		act, ok := actual[k]
		if !ok {
			return false
		}
		if !valuesEqual(k, exp, act) {
			return false
		}
	}
	return true
}

var nonDigit = regexp.MustCompile(`\D+`)

func valuesEqual(key string, a, b interface{}) bool {
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	aStr = strings.TrimSpace(aStr)
	bStr = strings.TrimSpace(bStr)

	// Phone: compare digits only.
	if key == "customer_phone" {
		return nonDigit.ReplaceAllString(aStr, "") == nonDigit.ReplaceAllString(bStr, "")
	}

	// Default: case-insensitive trimmed compare.
	return strings.EqualFold(aStr, bStr)
}

// SlotDiff returns a human-readable list of slot mismatches for failure reports.
func SlotDiff(expected, actual map[string]interface{}) []string {
	var out []string
	for k, exp := range expected {
		act, ok := actual[k]
		if !ok {
			out = append(out, fmt.Sprintf("%s: missing (expected %q)", k, exp))
			continue
		}
		if !valuesEqual(k, exp, act) {
			out = append(out, fmt.Sprintf("%s: expected %q, got %q", k, exp, act))
		}
	}
	return out
}
