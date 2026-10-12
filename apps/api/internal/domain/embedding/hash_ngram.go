package domainembedding

import "math"

const (
	HashNgramModel     = "hash-ngram-v1"
	HashNgramDimension = 128
)

// CosineSimilarity 计算两个已归一化或未归一化向量的余弦相似度。
func CosineSimilarity(left []float64, right []float64) (float64, error) {
	if len(left) != len(right) {
		return 0, ErrDimensionMismatch
	}
	var dot float64
	var leftNorm float64
	var rightNorm float64
	for i := range left {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)), nil
}
