// events.go holds the episodic event log: appending events (LogEvent) and the
// fused keyword + semantic recall path (RecallEvents). See episodic.go for the
// package overview.

package episodic

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
)

// rrfK is the reciprocal-rank-fusion constant used to merge the keyword and
// semantic result lists. 60 is the value from the original RRF paper; it damps
// the influence of rank position so neither list dominates.
const rrfK = 60

// Event is one entry in the episodic log — a single thing an agent did. The
// JSON field names are part of devcore-api's wire contract (cmd/devcore-api):
// renaming a field renames its API column.
type Event struct {
	ID      int64  `json:"id"`      // assigned by the store on insert; ignored as LogEvent input
	TS      string `json:"ts"`      // RFC3339 timestamp of when the event occurred
	Agent   string `json:"agent"`   // the agent that produced the event
	TaskID  string `json:"task_id"` // the task this event belongs to, or "" if none
	RunID   string `json:"run_id"`  // the run this event belongs to, or "" if none
	Type    string `json:"type"`    // decision | action | correction | learning | error | note
	Summary string `json:"summary"` // one-line description; scored for keyword recall
	Detail  string `json:"detail"`  // optional longer description; also scored
	Refs    string `json:"refs"`    // optional JSON array of file or commit references
}

// Hit is one event returned by RecallEvents, with its fused relevance score
// and which search produced it.
type Hit struct {
	Event  Event
	Score  float64
	Source string // "keyword", "semantic", or "both"
}

// record is an event paired with its decoded embedding, held in memory only
// for the duration of a recall.
type record struct {
	event     Event
	embedding []float32
}

// LogEvent appends e to the log with its embedding and returns the new event
// ID. embedding must have length VectorDim.
func (s *Store) LogEvent(ctx context.Context, e Event, embedding []float32) (int64, error) {
	if len(embedding) != VectorDim {
		return 0, fmt.Errorf("embedding has %d dimensions, want %d", len(embedding), VectorDim)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO events(ts, agent, task_id, run_id, type, summary, detail, refs, embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.TS, e.Agent, e.TaskID, e.RunID, e.Type, e.Summary, e.Detail, e.Refs,
		encodeVector(embedding))
	if err != nil {
		return 0, fmt.Errorf("inserting event: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading new event id: %w", err)
	}
	return id, nil
}

// ListEvents returns the most recent events, newest first. limit caps the
// number of rows returned and must be positive. ListEvents is the simple
// read path for UIs — RecallEvents is the ranked search path.
func (s *Store) ListEvents(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ts, agent, task_id, run_id, type, summary, detail, refs
		 FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.TS, &e.Agent, &e.TaskID, &e.RunID,
			&e.Type, &e.Summary, &e.Detail, &e.Refs); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading events: %w", err)
	}
	return events, nil
}

// RecallEvents returns events relevant to queryText, ranked by a fusion of
// keyword and semantic search. queryEmbedding must have length VectorDim;
// limit caps the number of results and must be positive.
func (s *Store) RecallEvents(
	ctx context.Context, queryText string, queryEmbedding []float32, limit int,
) ([]Hit, error) {
	if len(queryEmbedding) != VectorDim {
		return nil, fmt.Errorf("query embedding has %d dimensions, want %d", len(queryEmbedding), VectorDim)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}

	records, err := s.allRecords(ctx)
	if err != nil {
		return nil, err
	}

	keywordIDs := rankByKeyword(records, queryText, limit)
	semanticIDs, err := rankBySemantic(records, queryEmbedding, limit)
	if err != nil {
		return nil, err
	}

	events := make(map[int64]Event, len(records))
	for _, r := range records {
		events[r.event.ID] = r.event
	}
	ranked := fuse(keywordIDs, semanticIDs, limit)
	hits := make([]Hit, 0, len(ranked))
	for _, r := range ranked {
		hits = append(hits, Hit{Event: events[r.id], Score: r.score, Source: r.source})
	}
	return hits, nil
}

// allRecords loads every event with its decoded embedding.
func (s *Store) allRecords(ctx context.Context) ([]record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ts, agent, task_id, run_id, type, summary, detail, refs, embedding FROM events`)
	if err != nil {
		return nil, fmt.Errorf("loading events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []record
	for rows.Next() {
		var e Event
		var blob []byte
		if err := rows.Scan(&e.ID, &e.TS, &e.Agent, &e.TaskID, &e.RunID,
			&e.Type, &e.Summary, &e.Detail, &e.Refs, &blob); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}
		vec, err := decodeVector(blob)
		if err != nil {
			return nil, fmt.Errorf("decoding embedding for event %d: %w", e.ID, err)
		}
		records = append(records, record{event: e, embedding: vec})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading events: %w", err)
	}
	return records, nil
}

// rankByKeyword scores each record by how many distinct query tokens appear in
// its summary and detail, and returns up to limit event IDs, best score first.
// Records with no token match are excluded; an empty query yields no hits.
func rankByKeyword(records []record, queryText string, limit int) []int64 {
	queryTokens := tokenize(queryText)
	if len(queryTokens) == 0 {
		return nil
	}
	type scored struct {
		id    int64
		score int
	}
	var hits []scored
	for _, r := range records {
		text := tokenize(r.event.Summary + " " + r.event.Detail)
		score := 0
		for tok := range queryTokens {
			if text[tok] {
				score++
			}
		}
		if score > 0 {
			hits = append(hits, scored{id: r.event.ID, score: score})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].id < hits[j].id
	})
	ids := make([]int64, len(hits))
	for i, h := range hits {
		ids[i] = h.id
	}
	return trim(ids, limit)
}

// rankBySemantic returns up to limit event IDs ordered by ascending squared L2
// distance from queryEmbedding (nearest first).
func rankBySemantic(records []record, queryEmbedding []float32, limit int) ([]int64, error) {
	type scored struct {
		id       int64
		distance float64
	}
	hits := make([]scored, 0, len(records))
	for _, r := range records {
		dist, err := vectorDistanceSquared(queryEmbedding, r.embedding)
		if err != nil {
			return nil, fmt.Errorf("comparing embeddings for event %d: %w", r.event.ID, err)
		}
		hits = append(hits, scored{id: r.event.ID, distance: dist})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].distance != hits[j].distance {
			return hits[i].distance < hits[j].distance
		}
		return hits[i].id < hits[j].id
	})
	ids := make([]int64, len(hits))
	for i, h := range hits {
		ids[i] = h.id
	}
	return trim(ids, limit), nil
}

// trim returns the first limit elements of ids, or all of them if there are
// fewer.
func trim(ids []int64, limit int) []int64 {
	if len(ids) > limit {
		return ids[:limit]
	}
	return ids
}

// ranked is one fused search result before its event row is attached.
type ranked struct {
	id     int64
	score  float64
	source string
}

// fuse merges the keyword and semantic ID lists with reciprocal rank fusion and
// returns the top results, highest score first. Each input list is ordered
// best-match-first.
func fuse(keyword, semantic []int64, limit int) []ranked {
	score := map[int64]float64{}
	source := map[int64]string{}

	for rank, id := range keyword {
		score[id] += 1.0 / float64(rrfK+rank+1)
		source[id] = "keyword"
	}
	for rank, id := range semantic {
		score[id] += 1.0 / float64(rrfK+rank+1)
		if source[id] == "keyword" {
			source[id] = "both"
		} else {
			source[id] = "semantic"
		}
	}

	out := make([]ranked, 0, len(score))
	for id, sc := range score {
		out = append(out, ranked{id: id, score: sc, source: source[id]})
	}
	// Sort by score descending, breaking ties by ID for deterministic output.
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].id < out[j].id
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// tokenize splits text into a set of lowercase alphanumeric tokens.
func tokenize(text string) map[string]bool {
	set := map[string]bool{}
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !isAlphaNumeric(r)
	})
	for _, tok := range fields {
		set[tok] = true
	}
	return set
}

// isAlphaNumeric reports whether r is an ASCII letter or digit. tokenize lower-
// cases its input first, so only lowercase letters need to be accepted.
func isAlphaNumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// vectorDistanceSquared returns the squared L2 distance between two equal-length
// vectors. The square root is skipped because only the ordering matters.
func vectorDistanceSquared(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector lengths differ: %d and %d", len(a), len(b))
	}
	var sum float64
	for i := range a {
		diff := float64(a[i]) - float64(b[i])
		sum += diff * diff
	}
	return sum, nil
}

// encodeVector packs a vector into little-endian float32 bytes for BLOB storage.
func encodeVector(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeVector reverses encodeVector. It errors unless the blob is exactly
// VectorDim float32 values wide.
func decodeVector(blob []byte) ([]float32, error) {
	if len(blob) != VectorDim*4 {
		return nil, fmt.Errorf("embedding blob is %d bytes, want %d", len(blob), VectorDim*4)
	}
	vec := make([]float32, VectorDim)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vec, nil
}
