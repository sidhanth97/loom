// Copyright 2026 Teradata
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	llmtypes "github.com/teradata-labs/loom/pkg/llm/types"
	"github.com/teradata-labs/loom/pkg/shuttle"
)

// runCountingTool counts its runs.
type runCountingTool struct {
	schemaTool
	runs atomic.Int32
}

func (t *runCountingTool) Execute(_ context.Context, _ map[string]interface{}) (*shuttle.Result, error) {
	t.runs.Add(1)
	return &shuttle.Result{Success: true, Data: "ok"}, nil
}

// A call the reply's output limit cut off is not run on what arrived of it:
// the model is told why, so it sends the call again instead of puzzling over
// an error about a missing argument. A complete call in the same reply runs.
func TestATruncatedCallIsNotRunAndTheModelIsToldWhy(t *testing.T) {
	needs := &runCountingTool{schemaTool: schemaTool{name: "delegate", required: []string{"action"},
		props: map[string]*shuttle.JSONSchema{"action": {Type: "string"}}}}
	other := &runCountingTool{schemaTool: schemaTool{name: "note", required: []string{"text"},
		props: map[string]*shuttle.JSONSchema{"text": {Type: "string"}}}}
	llm := &scriptedStopReasonLLM{responses: []scriptedResponse{{
		toolCalls: []llmtypes.ToolCall{
			{ID: "ok", Name: "note", Input: map[string]interface{}{"text": "hi"}},
			{ID: "cut", Name: "delegate", Input: map[string]interface{}{}},
		},
		stopReason: "max_tokens",
	}}}
	ag := newOutputCBAgent(t, 3, llm, needs, other)
	_, err := ag.Chat(context.Background(), "s-cut", "go")
	require.NoError(t, err)

	require.Zero(t, needs.runs.Load(), "the cut-off call was run")
	require.EqualValues(t, 1, other.runs.Load(), "the complete call in the same reply did not run")
	var told string
	for _, m := range ag.memory.GetOrCreateSessionWithAgent(context.Background(), "s-cut", ag.config.Name, "").GetMessages() {
		if m.Role == "tool" && m.ToolUseID == "cut" {
			told = m.Content
		}
	}
	require.True(t, strings.Contains(told, "output limit"), "the model was not told why: %q", told)
}
