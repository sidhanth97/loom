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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compacting a session between turns is the in-loop fold asked for now: the
// agent is sent its settled context with the fold ask and the person's focus,
// its summary replaces the past on the session's rows, and the next turn goes
// on from the summary. A session with nothing to fold says so.
func TestCompactFoldsTheSessionOnRequest(t *testing.T) {
	model := &foldingStream{steps: 6}
	ag, store := foldingAgent(t, model)
	ctx := context.Background()

	require.ErrorIs(t, ag.Compact(ctx, "s-none", ""), ErrNothingToCompact, "a session with no history was compacted")

	_, err := ag.Chat(ctx, "s-compact", "do the job")
	require.NoError(t, err)
	require.Zero(t, model.folds, "the turn folded on its own; the test does not reach Compact")
	before := len(ag.memory.GetOrCreateSessionWithAgent(ctx, "s-compact", ag.config.Name, "").GetMessages())

	require.NoError(t, ag.Compact(ctx, "s-compact", "the widget schema"))
	require.Equal(t, 1, model.folds)
	ask := model.sent[len(model.sent)-1]
	last := ask[len(ask)-1].Content
	assert.Contains(t, last, "Context maintenance: summarise the work so far")
	assert.Contains(t, last, "keep in view: the widget schema", "the person's focus did not reach the fold ask")

	sess := ag.memory.GetOrCreateSessionWithAgent(ctx, "s-compact", ag.config.Name, "")
	segMem := sess.SegmentedMem.(*SegmentedMemory)
	assert.Equal(t, "STATE-OF-WORK-SUMMARY", segMem.summary.text)
	assert.Less(t, len(sess.GetMessages()), before, "nothing was folded away")
	// The fold is on the session's rows: an agent that loads it afresh starts
	// from the summary.
	fresh := NewAgent(&mockBackend{}, model, WithConfig(ag.config), WithMemory(NewMemoryWithStore(store)))
	reloaded := fresh.memory.GetOrCreateSessionWithAgent(ctx, "s-compact", ag.config.Name, "")
	assert.Equal(t, "STATE-OF-WORK-SUMMARY", reloaded.SegmentedMem.(*SegmentedMemory).summary.text, "the summary is not on the session's rows")

	require.ErrorIs(t, ag.Compact(ctx, "s-compact", ""), ErrNothingToCompact, "a fresh fold was compacted again")

	_, err = ag.Chat(ctx, "s-compact", "carry on")
	require.NoError(t, err)
	next := model.sent[len(model.sent)-1]
	found := false
	for _, m := range next {
		if strings.Contains(m.Content, "STATE-OF-WORK-SUMMARY") {
			found = true
		}
	}
	assert.True(t, found, "the next turn was not sent the summary")
}
