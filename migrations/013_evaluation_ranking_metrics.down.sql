ALTER TABLE eval_results
DROP COLUMN IF EXISTS ndcg_at_k,
DROP COLUMN IF EXISTS hit_rate_at_k;

ALTER TABLE eval_runs
DROP COLUMN IF EXISTS avg_ndcg_at_k,
DROP COLUMN IF EXISTS avg_hit_rate_at_k;
