// Copyright 2026 Teradata
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teradata-labs/loom/pkg/shuttle"
	llmtypes "github.com/teradata-labs/loom/pkg/types"
)

// summaryModel answers every call with a summary and keeps what it was asked.
type summaryModel struct {
	mu    sync.Mutex
	asked []string
}

func (s *summaryModel) Name() string  { return "summary" }
func (s *summaryModel) Model() string { return "summary-model" }
func (s *summaryModel) Chat(_ context.Context, msgs []Message, _ []shuttle.Tool) (*LLMResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = append(s.asked, msgs[len(msgs)-1].Content)
	return &LLMResponse{Content: `{"summary": "SUMMARY-BY-FAST"}`, StopReason: "end_turn",
		Usage: llmtypes.Usage{InputTokens: 50, OutputTokens: 5, TotalTokens: 55, CostUSD: callCost}}, nil
}

// testCompaction picks the summary model when it holds the material, and
// keeps what it was told.
type testCompaction struct {
	window, fastWindow int
	fast               LLMProvider
	mu                 sync.Mutex
	done               []Compacted
}

func (c *testCompaction) Window(context.Context, LLMProvider) int { return c.window }
func (c *testCompaction) Summarizer(_ context.Context, _ LLMProvider, tokens int) (LLMProvider, string) {
	if c.fastWindow >= tokens {
		return c.fast, "fast"
	}
	return nil, "advanced"
}
func (c *testCompaction) Compacted(_ context.Context, done Compacted) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.done = append(c.done, done)
}

// The material a compaction summarises names every tool call but carries none
// of what the calls returned.
func TestCompactionMaterialLeavesToolOutputsOut(t *testing.T) {
	sm := NewSegmentedMemory("ROM", 200000, 20000)
	sm.SetProtectedRecentTurns(2)
	seedInloopSession(sm)
	material, before := sm.CompactionMaterial()
	require.NotEmpty(t, material)
	assert.Contains(t, material, "the JOB")
	assert.Contains(t, material, "[tool call] shell_execute")
	assert.NotContains(t, material, strings.Repeat("r", 100), "a tool output reached the compaction")
	assert.Positive(t, before)
}

// With an embedder's compaction, a turn that grows past its share of the
// window is compacted by the model it picks — the agent never summarises
// itself — and the summary replaces the past before the turn goes on.
func TestCompactionSummarisesWithTheModelPickedForItsSize(t *testing.T) {
	model := &foldingStream{steps: 24}
	ag, _ := foldingAgent(t, model)
	fast := &summaryModel{}
	c := &testCompaction{window: 12000, fastWindow: 1_000_000, fast: fast}
	ag.compaction = c

	resp, err := ag.Chat(context.Background(), "s-compaction", "do the long job")
	require.NoError(t, err)
	require.Equal(t, "final answer", resp.Content)
	assert.Zero(t, model.folds, "the agent was asked to fold itself")
	require.NotEmpty(t, c.done, "the turn never compacted")
	assert.Equal(t, "fast", c.done[0].By)
	assert.False(t, c.done[0].Manual)
	assert.Less(t, c.done[0].After, c.done[0].Before)
	require.NotEmpty(t, fast.asked)
	assert.Contains(t, fast.asked[0], "Tool outputs have been left out")

	// The session model goes on from the summary.
	last := model.sent[len(model.sent)-1]
	var joined strings.Builder
	for _, m := range last {
		joined.WriteString(m.Content)
	}
	assert.Contains(t, joined.String(), "SUMMARY-BY-FAST")
}

// Material larger than the Fast model's window is summarised by the session's
// own model, and a compaction asked for between turns says so.
func TestCompactionFallsBackToTheSessionModelAndCompactUsesIt(t *testing.T) {
	model := &foldingStream{steps: 6}
	ag, _ := foldingAgent(t, model)
	fast := &summaryModel{}
	c := &testCompaction{window: 1_000_000, fastWindow: 10, fast: fast}
	ag.compaction = c
	ctx := context.Background()

	_, err := ag.Chat(ctx, "s-manual", "do the job")
	require.NoError(t, err)
	require.Empty(t, c.done, "the turn compacted under its window")

	require.NoError(t, ag.Compact(ctx, "s-manual", "the widget schema"))
	require.Len(t, c.done, 1)
	assert.Equal(t, "advanced", c.done[0].By)
	assert.True(t, c.done[0].Manual)
	assert.Empty(t, fast.asked, "material too large for the Fast model was sent to it")
	assert.Equal(t, 1, model.folds, "the session model did not write the summary")
	ask := model.sent[len(model.sent)-1]
	assert.Contains(t, ask[len(ask)-1].Content, "keep in view: the widget schema")
}
