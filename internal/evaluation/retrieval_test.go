package evaluation

import (
	"math"
	"testing"

	"github.com/google/uuid"
)

func TestCalculateRankingMetrics(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	c := uuid.New()

	tests := []struct {
		name        string
		expected    []uuid.UUID
		retrieved   []uuid.UUID
		k           int
		wantHitRate float64
		wantNDCG    float64
	}{
		{
			name:        "relevant chunk at rank one",
			expected:    []uuid.UUID{a},
			retrieved:   []uuid.UUID{a, b, c},
			k:           3,
			wantHitRate: 1,
			wantNDCG:    1,
		},
		{
			name:        "no relevant chunks retrieved",
			expected:    []uuid.UUID{a},
			retrieved:   []uuid.UUID{b, c},
			k:           2,
			wantHitRate: 0,
			wantNDCG:    0,
		},
		{
			name:        "relevant chunk at rank two",
			expected:    []uuid.UUID{a},
			retrieved:   []uuid.UUID{b, a},
			k:           2,
			wantHitRate: 1,
			wantNDCG:    1 / math.Log2(3),
		},
		{
			name:        "multiple relevant chunks in ideal order",
			expected:    []uuid.UUID{a, b},
			retrieved:   []uuid.UUID{a, b, c},
			k:           3,
			wantHitRate: 1,
			wantNDCG:    1,
		},
		{
			name:        "duplicate retrieved chunk counts once",
			expected:    []uuid.UUID{a, b},
			retrieved:   []uuid.UUID{a, a, b},
			k:           3,
			wantHitRate: 1,
			wantNDCG: (1 + 1/math.Log2(4)) /
				(1 + 1/math.Log2(3)),
		},
		{
			name:        "empty expected set",
			expected:    nil,
			retrieved:   []uuid.UUID{a, b},
			k:           2,
			wantHitRate: 0,
			wantNDCG:    0,
		},
		{
			name:        "empty retrieved results",
			expected:    []uuid.UUID{a},
			retrieved:   nil,
			k:           3,
			wantHitRate: 0,
			wantNDCG:    0,
		},
		{
			name:        "non-positive k",
			expected:    []uuid.UUID{a},
			retrieved:   []uuid.UUID{a},
			k:           0,
			wantHitRate: 0,
			wantNDCG:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHitRate, gotNDCG := calculateRankingMetrics(
				tt.expected,
				tt.retrieved,
				tt.k,
			)

			if gotHitRate != tt.wantHitRate {
				t.Errorf(
					"HitRate@K = %v, want %v",
					gotHitRate,
					tt.wantHitRate,
				)
			}

			if math.Abs(gotNDCG-tt.wantNDCG) > 1e-9 {
				t.Errorf(
					"NDCG@K = %v, want %v",
					gotNDCG,
					tt.wantNDCG,
				)
			}
		})
	}
}
