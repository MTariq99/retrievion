package evaluation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/llm"
)

type FaithfulnessResult struct {
	Score     float64 // 0.0–1.0
	Reasoning string
	Supported bool // score >= threshold
}

type judgeResponse struct {
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

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
		Supported: parsed.Score >= consts.FaithfulnessThreshold,
	}, nil
}

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

func CompareToBaseline(currentAvgFaithfulness, baselineAvgFaithfulness float64, tolerancePct float64) (regressed bool, deltaPct float64) {
	if baselineAvgFaithfulness == 0 {
		return false, 0
	}
	deltaPct = (currentAvgFaithfulness - baselineAvgFaithfulness) / baselineAvgFaithfulness * 100
	regressed = deltaPct < -tolerancePct
	return regressed, deltaPct
}
