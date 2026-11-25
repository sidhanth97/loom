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

// This file implements compaction an embedder steers: the fold, run when the
// context reaches a share of the session model's own window, summarised by a
// model the embedder picks from what is to be summarised, with tool outputs
// left out so the summary is cheap to make.
package agent

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// Compaction lets an embedder decide how a session's context is compacted.
// With one set, a fold runs when the context reaches CompactionStartPercent of
// Window. The settled past a fold removes is rendered without its tool
// outputs, summarised by the model Summarizer picks for its size, and replaced
// by the summary; the session's own model then goes on. Without one, the agent
// summarises itself at loom's own mark (the in-loop fold).
type Compaction interface {
	// Window is the context window, in tokens, of own — the model the agent
	// runs on; 0 when unknown, and loom's configured window is used.
	Window(ctx context.Context, own LLMProvider) int
	// Summarizer is the model to summarise tokens of context with, and its name
	// for the record, such as a model class. A nil model is own.
	Summarizer(ctx context.Context, own LLMProvider, tokens int) (LLMProvider, string)
	// Compacted is told of each compaction that committed.
	Compacted(ctx context.Context, c Compacted)
}

// Compacted is one compaction that committed.
type Compacted struct {
	SessionID string
	// By is the name Summarizer gave the model that wrote the summary.
	By string
	// Before and After are the context's size, in tokens, around the compaction.
	Before, After int
	// Manual is a compaction asked for between turns (Compact).
	Manual bool
}

// CompactionStartPercent is the share of the session model's window at which
// an embedder's compaction runs.
const CompactionStartPercent = 90

// WithCompaction hands compaction to c.
func WithCompaction(c Compaction) Option {
	return func(a *Agent) { a.compaction = c }
}

// compactAsk is what the summarising model is sent; the material goes last.
const compactAsk = `You maintain the working memory of an AI agent part-way through a task. Below is the summary it had so far, if any, and the part of its conversation that is about to be removed from its context. Tool outputs have been left out; each tool call is listed by name and input.

Write the summary that will replace both. Reply with ONLY this JSON — no other text:

{"summary": "<the state of the work>"}

Write for one reader: the agent, continuing this work with nothing but this summary and its most recent turns. Write the current state in the present tense, never the story of how it got here. Keep, in this order: the operator's instructions and rulings, verbatim where the wording carries force; decisions with their rationale; approaches ruled out, each with its reason; constraints and facts discovered about this environment (schemas, conventions, paths, gotchas); where the work stands — done, in hand, next. Identifiers and paths as pointers, never file contents or query results. Carry forward everything in the previous summary that still serves the work: whatever you omit is gone for good. Drop superseded values, corrected errors and dead ends that closed no door. Do not invent facts.`

// compactionLimit is the context size, in tokens, at which the embedder's
// compaction runs for this session.
func (a *Agent) compactionLimit(ctx context.Context, sm *SegmentedMemory) int {
	window := a.compaction.Window(ctx, a.llm)
	if window <= 0 {
		window = sm.ContextWindow()
	}
	return window * CompactionStartPercent / 100
}

// compactWith folds sm with the embedder's compaction: the past a fold removes,
// tool outputs left out, is summarised by the model it picks, and the summary
// replaces that past. focus, when given, is what the person wants kept in view.
// Returns whether the fold committed and what its calls cost.
func (a *Agent) compactWith(ctx context.Context, sessionID string, sm *SegmentedMemory, focus string, manual bool) (bool, Usage, error) {
	var spent Usage
	material, before := sm.CompactionMaterial()
	if material == "" {
		return false, spent, ErrNothingToCompact
	}
	ask := compactAsk
	if f := strings.TrimSpace(focus); f != "" {
		ask += "\n\nThe operator asked this summary to keep in view: " + f
	}
	ask += "\n\n" + material
	model, by := a.compaction.Summarizer(ctx, a.llm, GetTokenCounter().CountTokens(ask))
	if model == nil {
		model = a.llm
	}
	// A reply that is not a summary is asked again once.
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := model.Chat(ctx, []Message{{Role: "user", Content: ask}}, nil)
		if err != nil {
			return false, spent, fmt.Errorf("compaction by %s: %w", by, err)
		}
		addUsage(&spent, resp.Usage)
		summary, ok := parseFoldReply(resp.Content)
		if !ok {
			continue
		}
		applied, after := sm.ApplyFoldReply(ctx, summary)
		if !applied {
			return false, spent, fmt.Errorf("compaction: the summary could not be applied")
		}
		zap.L().Info("compaction: applied",
			zap.String("session_id", sessionID), zap.String("by", by),
			zap.Int("before_tokens", before), zap.Int("after_tokens", after), zap.Bool("manual", manual))
		a.compaction.Compacted(ctx, Compacted{SessionID: sessionID, By: by, Before: before, After: after, Manual: manual})
		return true, spent, nil
	}
	return false, spent, fmt.Errorf("compaction: %s did not reply with a summary", by)
}
