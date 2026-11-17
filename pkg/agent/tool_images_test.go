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
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	llmtypes "github.com/teradata-labs/loom/pkg/llm/types"
	"github.com/teradata-labs/loom/pkg/shuttle"
	"github.com/teradata-labs/loom/pkg/types"
)

// pictureTool returns a picture with its text result.
type pictureTool struct{ schemaTool }

func (t *pictureTool) Execute(_ context.Context, _ map[string]interface{}) (*shuttle.Result, error) {
	return &shuttle.Result{Success: true, Data: map[string]interface{}{"path": "chart.png"}, Metadata: map[string]interface{}{
		ToolImagesKey: []types.ImageContent{{Type: "image", Source: types.ImageSource{Type: "base64", MediaType: "image/png", Data: "aGVsbG8="}}},
	}}, nil
}

// seeingLLM calls the picture tool once, then records what it is sent.
type seeingLLM struct {
	mu   sync.Mutex
	sent [][]llmtypes.Message
}

func (l *seeingLLM) Chat(_ context.Context, msgs []llmtypes.Message, _ []shuttle.Tool) (*llmtypes.LLMResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, append([]llmtypes.Message(nil), msgs...))
	if len(l.sent) == 1 {
		return &llmtypes.LLMResponse{ToolCalls: []llmtypes.ToolCall{{ID: "see", Name: "look", Input: map[string]interface{}{}}},
			StopReason: "tool_use", Usage: llmtypes.Usage{InputTokens: 10, OutputTokens: 10}}, nil
	}
	return &llmtypes.LLMResponse{Content: "a chart", StopReason: "end_turn", Usage: llmtypes.Usage{InputTokens: 10, OutputTokens: 10}}, nil
}

func (l *seeingLLM) Name() string  { return "seeing" }
func (l *seeingLLM) Model() string { return "seeing-v1" }

// A tool's pictures reach the model as image blocks right after the batch's
// results, and the tool's own row keeps none of their bytes.
func TestAToolsPicturesReachTheModelAfterItsResult(t *testing.T) {
	llm := &seeingLLM{}
	cfg := DefaultConfig()
	cfg.PatternConfig = DefaultPatternConfig()
	cfg.PatternConfig.UseLLMClassifier = false
	ag := NewAgent(&mockBackend{}, llm, WithConfig(cfg))
	ag.RegisterTool(&pictureTool{schemaTool: schemaTool{name: "look"}})
	_, err := ag.Chat(context.Background(), "s-pic", "look at it")
	require.NoError(t, err)
	require.Len(t, llm.sent, 2)

	second := llm.sent[1]
	resultAt, picturesAt := -1, -1
	for i, m := range second {
		if m.Role == "tool" && m.ToolUseID == "see" {
			resultAt = i
			require.NotContains(t, m.Content, "aGVsbG8=", "the picture's bytes are in the tool's text result")
		}
		for _, b := range m.ContentBlocks {
			if b.Type == "image" && b.Image != nil && b.Image.Source.Data == "aGVsbG8=" {
				picturesAt = i
				require.Equal(t, "user", m.Role, "the picture does not reach the model as user content")
			}
		}
	}
	require.GreaterOrEqual(t, resultAt, 0, "the tool's result is not in the next call")
	require.Greater(t, picturesAt, resultAt, "the picture does not follow the tool's result")
}
