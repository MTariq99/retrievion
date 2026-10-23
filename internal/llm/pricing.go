package llm

import "github.com/mtariq99/retrievion/models"

func NewPricing(cfg *models.Config) PricePerMillionTokens {
	return PricePerMillionTokens{
		Input:  cfg.LLM.InputPricePerMillion,
		Output: cfg.LLM.OutputPricePerMillion,
	}
}
