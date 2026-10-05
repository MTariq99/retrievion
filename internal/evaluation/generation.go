package evaluation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mtariq99/retrievion/internal/llm"
)

// FaithfulnessResult captures not just a score but the JUDGE'S REASONING
// — Principle 11 stated explicitly: "LLM-as-judge is useful but not
// automatically ground truth." A bare float with no explanation is
// unauditable; storing the reasoning (eval_results.judge_reasoning in
// the migration) is what lets a human spot-check whether the judge
// itself is behaving sensibly, which you must do periodically.
type FaithfulnessResult struct {
	Score     float64 // 0.0–1.0
	Reasoning string
	Supported bool // score >= threshold
}

const faithfulnessThreshold = 0.7

// judgeResponseSchema mirrors Topic 4's structured-output discipline —
// the judge's output is CONSTRAINED to a known shape, never parsed from
// free text, for exactly the same reasons Topic 4 argued: shape
// guarantees let you trust the parse, never the semantic correctness.
type judgeResponse struct {
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

// EvaluateFaithfulness asks a SEPARATE LLM call whether `answer` is
// actually supported by `retrievedContext` — this is the concrete
// implementation of Topic 10's grounding concept, now measured rather
// than assumed.
//
// Deliberately distinct failure modes this catches, per the lesson's
// explicit requirement:
//   - "Answer looks correct BUT context doesn't support it" → LOW score,
//     even if the answer happens to be factually true — because THIS
//     particular system didn't ground it in evidence, which is what
//     RAG exists to guarantee (Topic 10, Section 3.5).
//   - "Context is correct BUT answer is wrong" → this function alone
//     can't fully distinguish this from the case above; pair with a
//     separate answer-relevance check against expected_answer for that.
func EvaluateFaithfulness(ctx context.Context, llmClient *llm.Client, question, answer string, retrievedContext []string) (FaithfulnessResult, error) {
	contextBlock := ""
	for i, c := range retrievedContext {
		contextBlock += fmt.Sprintf("[%d] %s\n\n", i+1, c)
	}

	judgePrompt := fmt.Sprintf(`You are evaluating whether an AI-generated answer is
FAITHFUL to the provided context — meaning every claim in the answer is
actually supported by the context, not from outside knowledge.

Context:
%s

Question: %s

Answer to evaluate: %s

Score from 0.0 (completely unsupported/hallucinated) to 1.0 (fully
grounded in the context). Return ONLY JSON matching this schema:
{"score": <float>, "reasoning": "<one sentence explaining the score>"}`,
		contextBlock, question, answer)

	// Reuses your existing internal/llm/client.go — the judge call is
	// STRUCTURALLY IDENTICAL to any other LLM call in the system, just
	// with a schema-constrained response (Topic 4) and low temperature
	// (this is a judgment task requiring consistency, not creativity —
	// Topic 1, Section 3.3's temperature reasoning applies directly).
	raw, err := llmClient.GenerateStructured(ctx, judgePrompt, judgeResponseSchema())
	if err != nil {
		return FaithfulnessResult{}, fmt.Errorf("judge call failed: %w", err)
	}

	var parsed judgeResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return FaithfulnessResult{}, fmt.Errorf("failed to parse judge response: %w", err)
	}

	return FaithfulnessResult{
		Score:     parsed.Score,
		Reasoning: parsed.Reasoning,
		Supported: parsed.Score >= faithfulnessThreshold,
	}, nil
}

// judgeResponseSchema — adjust to match whatever structured-output
// schema type your internal/llm package's GenerateStructured expects
// (mirrors Topic 4's schema pattern applied to Gemini's responseSchema).
func judgeResponseSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"score":     map[string]interface{}{"type": "number"},
			"reasoning": map[string]interface{}{"type": "string"},
		},
		"required": []string{"score", "reasoning"},
	}
}

// CompareToBaseline is the regression-detection step (Section 8's
// CI/CD gate) — pass/fail against a stored baseline, not an absolute
// threshold alone, since "good enough" quality is relative to what you
// previously shipped.
func CompareToBaseline(currentAvgFaithfulness, baselineAvgFaithfulness float64, tolerancePct float64) (regressed bool, deltaPct float64) {
	if baselineAvgFaithfulness == 0 {
		return false, 0 // no baseline yet — nothing to compare against
	}
	deltaPct = (currentAvgFaithfulness - baselineAvgFaithfulness) / baselineAvgFaithfulness * 100
	regressed = deltaPct < -tolerancePct // a DROP beyond tolerance is a regression
	return regressed, deltaPct
}
