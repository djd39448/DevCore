// Internal (white-box) tests for the episodic package's pure helper functions:
// vector encoding, distance, recall fusion, and tokenisation. These carry the
// package's hardest logic, so they are tested directly here rather than only
// transitively through RecallEvents.
package episodic

import (
	"math"
	"testing"
)

func TestEncodeDecodeVectorRoundTrip(t *testing.T) {
	t.Parallel()
	original := make([]float32, VectorDim)
	for i := range original {
		original[i] = float32(i) * 0.001
	}

	decoded, err := decodeVector(encodeVector(original))
	if err != nil {
		t.Fatalf("decodeVector: %v", err)
	}
	if len(decoded) != VectorDim {
		t.Fatalf("decoded length = %d, want %d", len(decoded), VectorDim)
	}
	for i := range original {
		if decoded[i] != original[i] {
			t.Fatalf("round trip changed element %d: %v -> %v", i, original[i], decoded[i])
		}
	}
}

func TestDecodeVectorRejectsWrongSize(t *testing.T) {
	t.Parallel()
	if _, err := decodeVector([]byte{0x00, 0x01, 0x02}); err == nil {
		t.Fatal("decodeVector accepted a blob of the wrong size, want an error")
	}
}

func TestVectorDistanceSquared(t *testing.T) {
	t.Parallel()
	same := []float32{1, 2, 3}
	if d, err := vectorDistanceSquared(same, same); err != nil || d != 0 {
		t.Fatalf("distance to self = (%v, %v), want (0, nil)", d, err)
	}

	dist, err := vectorDistanceSquared([]float32{0, 0}, []float32{3, 4})
	if err != nil {
		t.Fatalf("vectorDistanceSquared: %v", err)
	}
	if math.Abs(dist-25) > 1e-9 {
		t.Fatalf("squared distance = %v, want 25", dist)
	}

	if _, err := vectorDistanceSquared([]float32{1}, []float32{1, 2}); err == nil {
		t.Fatal("vectorDistanceSquared accepted mismatched lengths, want an error")
	}
}

func TestFuseMergesAndRanks(t *testing.T) {
	t.Parallel()
	// Event 7 is in both lists, so it should rank first and be sourced "both".
	ranked := fuse([]int64{7, 1}, []int64{7, 2}, 10)
	if len(ranked) != 3 {
		t.Fatalf("fuse returned %d results, want 3", len(ranked))
	}
	if ranked[0].id != 7 || ranked[0].source != "both" {
		t.Fatalf("top result = (id %d, %s), want (7, both)", ranked[0].id, ranked[0].source)
	}

	if got := fuse([]int64{1, 2, 3}, nil, 2); len(got) != 2 {
		t.Fatalf("fuse with limit 2 returned %d results, want 2", len(got))
	}
}

func TestTokenize(t *testing.T) {
	t.Parallel()
	tokens := tokenize("Chose Claude-Code, the PROXY.")
	for _, want := range []string{"chose", "claude", "code", "the", "proxy"} {
		if !tokens[want] {
			t.Errorf("tokenize dropped the token %q", want)
		}
	}
	if len(tokenize("")) != 0 {
		t.Fatal("tokenize of an empty string should yield no tokens")
	}
}
