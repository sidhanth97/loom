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
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	llmtypes "github.com/teradata-labs/loom/pkg/llm/types"
	"github.com/teradata-labs/loom/pkg/observability"
	"github.com/teradata-labs/loom/pkg/shuttle"
)

// outageLLM answers from its script, failing the calls numbered in down.
type outageLLM struct {
	mockToolCallingLLM
	mu    sync.Mutex
	calls int
	down  map[int]bool
}

func (o *outageLLM) Chat(ctx context.Context, messages []llmtypes.Message, tools []shuttle.Tool) (*llmtypes.LLMResponse, error) {
	o.mu.Lock()
	o.calls++
	n := o.calls
	o.mu.Unlock()
	if o.down[n] {
		return nil, errors.New("provider unavailable")
	}
	return o.mockToolCallingLLM.Chat(ctx, messages, tools)
}

func newContinueAgent(t *testing.T, llm LLMProvider) *Agent {
	t.Helper()
	cfg := DefaultConfig()
	cfg.PatternConfig = DefaultPatternConfig()
	cfg.PatternConfig.UseLLMClassifier = false
	cfg.Retry.Enabled = false
	sessions, err := NewSessionStore(filepath.Join(t.TempDir(), "continue.db"), observability.NewNoOpTracer())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sessions.Close() })
	return NewAgent(&mockBackend{}, llm, WithConfig(cfg), WithMemory(NewMemoryWithStore(sessions)))
}

func countRole(ag *Agent, sessionID, role string) int {
	n := 0
	for _, m := range rawSessionMessages(ag.memory.GetOrCreateSessionWithAgent(context.Background(), sessionID, ag.config.Name, "")) {
		if m.Role == role {
			n++
		}
	}
	return n
}

// A turn the provider cut off after its tool ran carries on from there: the
// tool is not run again and no user message is added.
func TestContinueChatCarriesOnAfterAnOutage(t *testing.T) {
	llm := &outageLLM{down: map[int]bool{2: true}, mockToolCallingLLM: mockToolCallingLLM{responses: []mockLLMResponse{
		{toolCalls: []llmtypes.ToolCall{{ID: "c1", Name: "count", Input: map[string]interface{}{}}}},
		{content: "Counted once."},
	}}}
	ag := newContinueAgent(t, llm)
	tool := &countingTool{name: "count"}
	ag.RegisterTool(tool)

	if _, err := ag.Chat(context.Background(), "s1", "count it"); err == nil {
		t.Fatal("want the outage to fail the turn")
	}
	resp, err := ag.ContinueChat(context.Background(), "s1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Counted once." {
		t.Fatalf("reply = %q", resp.Content)
	}
	if n := tool.runs.Load(); n != 1 {
		t.Errorf("the tool ran %d times, want once", n)
	}
	if n := countRole(ag, "s1", "user"); n != 1 {
		t.Errorf("%d user rows, want 1: continuing adds no message", n)
	}
	if n := countAssistant(ag, "s1", "Counted once."); n != 1 {
		t.Errorf("the reply is stored %d times, want once", n)
	}

	if _, err := ag.ContinueChat(context.Background(), "s1", nil); !errors.Is(err, ErrNothingToContinue) {
		t.Errorf("a finished turn: got %v, want ErrNothingToContinue", err)
	}
}

// A call left without a result is not run again: it gets an interrupted
// result and the model decides.
func TestContinueChatInterruptsACallThatNeverReturned(t *testing.T) {
	llm := &mockToolCallingLLM{responses: []mockLLMResponse{{content: "It was cut off; I checked and stopped."}}}
	ag := newContinueAgent(t, llm)
	tool := &countingTool{name: "count"}
	ag.RegisterTool(tool)

	ctx := context.Background()
	sess := ag.memory.GetOrCreateSessionWithAgent(ctx, "s2", ag.config.Name, "")
	ag.appendMessage(ctx, sess, Message{Role: "user", Content: "count it", Timestamp: time.Now()}, true)
	ag.appendMessage(ctx, sess, Message{Role: "assistant", Timestamp: time.Now(),
		ToolCalls: []ToolCall{{ID: "c1", Name: "count", Input: map[string]interface{}{}}}}, false)
	if err := ag.memory.PersistSession(ctx, sess); err != nil {
		t.Fatal(err)
	}

	resp, err := ag.ContinueChat(ctx, "s2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "It was cut off; I checked and stopped." {
		t.Fatalf("reply = %q", resp.Content)
	}
	if n := tool.runs.Load(); n != 0 {
		t.Errorf("an interrupted call ran again %d times", n)
	}
	var interrupted bool
	for _, m := range rawSessionMessages(ag.memory.GetOrCreateSessionWithAgent(ctx, "s2", ag.config.Name, "")) {
		if m.Role == "tool" && m.ToolUseID == "c1" && m.ToolResult != nil && m.ToolResult.Error != nil {
			interrupted = m.ToolResult.Error.Code == InterruptedCallCode
		}
	}
	if !interrupted {
		t.Error("the call has no interrupted result")
	}
}

func TestContinueChatRefusesANewOrParkedSession(t *testing.T) {
	ag := newContinueAgent(t, &mockToolCallingLLM{})
	if _, err := ag.ContinueChat(context.Background(), "nobody", nil); !errors.Is(err, ErrNothingToContinue) {
		t.Errorf("no session: got %v, want ErrNothingToContinue", err)
	}

	f := newParkFixture(t, []shuttle.Hook{scopedAskHook{tool: "export_csv"}}, []mockLLMResponse{
		{toolCalls: []llmtypes.ToolCall{{ID: "c1", Name: "export_csv", Input: map[string]interface{}{"v": "w"}}}},
	}, "export_csv")
	var parked *TurnParkedError
	if _, err := f.ag.Chat(context.Background(), "p1", "go"); !errors.As(err, &parked) {
		t.Fatalf("want park, got %v", err)
	}
	var waiting *SessionParkedError
	if _, err := f.ag.ContinueChat(context.Background(), "p1", nil); !errors.As(err, &waiting) {
		t.Errorf("a parked turn: got %v, want SessionParkedError", err)
	}
}
