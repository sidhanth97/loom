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

// This file implements continuing an unfinished turn: the counterpart of
// ResumeChat for a turn that stopped on its own — an LLM outage, a crash, a
// cancellation — rather than one parked for a human. The turn's persisted rows
// are its state; ContinueChat re-enters the conversation loop on them without
// a new user message.
package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	mcpadapter "github.com/teradata-labs/loom/pkg/mcp/adapter"
	"github.com/teradata-labs/loom/pkg/observability"
	"github.com/teradata-labs/loom/pkg/session"
	"github.com/teradata-labs/loom/pkg/shuttle"
	"github.com/teradata-labs/loom/pkg/taskctx"
)

// ErrNothingToContinue: the session has no turn, or its last turn already
// ended with an answer.
var ErrNothingToContinue = errors.New("nothing to continue: the last turn is complete")

// InterruptedCallCode is the error code of the result ContinueChat records for
// a call that has no result because its turn stopped while it ran.
const InterruptedCallCode = "interrupted"

// ContinueChat carries on a session's last turn from where it stopped, with
// no new user message: the model is asked again with the turn's history as
// persisted. A call of the turn's last batch that left no result is not run
// again — it may have taken effect before the turn stopped — but gets an
// interrupted result, so the model decides whether to repeat it.
//
// A turn waiting on a human decision is refused with SessionParkedError: that
// turn is ResumeChat's to continue. The loop budget restarts, as on resume.
func (a *Agent) ContinueChat(ctx context.Context, sessionID string, progressCallback ProgressCallback) (*Response, error) {
	endTurn, err := a.takeTurn(ctx, sessionID, false)
	switch {
	case errors.Is(err, errNoSessionRow):
		return nil, ErrNothingToContinue
	case err != nil:
		return nil, err
	}
	defer endTurn()

	ctx = session.WithSessionID(ctx, sessionID)
	unlock := a.lockSession(sessionID)
	defer unlock()

	startTime := time.Now()
	ctx, span := a.tracer.StartSpan(ctx, "agent.conversation.continue")
	defer a.tracer.EndSpan(span)
	span.SetAttribute(observability.AttrSessionID, sessionID)

	sess := a.memory.GetOrCreateSessionWithAgent(ctx, sessionID, a.config.Name, "")
	if err := a.guardParkedTail(ctx, sessionID, sess); err != nil {
		span.AddEvent("continue.refused", map[string]interface{}{"reason": err.Error()})
		return nil, err
	}
	turnIndex, turnUserMessage, unfinished := unfinishedTurn(sess)
	if !unfinished {
		return nil, ErrNothingToContinue
	}
	span.SetAttribute("continue.turn", turnIndex)

	if progressCallback != nil {
		ctx = ContextWithProgressCallback(ctx, progressCallback)
	}
	// The turn's entry work (graph-memory injection and extraction) already ran.
	ctx = contextWithResumedTurn(ctx)
	ctx, taskBinding := taskctx.ContextWithBinding(ctx)
	ctx, handleCollector := mcpadapter.WithHandleCollector(ctx)
	parked := false
	defer func() {
		if !parked {
			a.leases.apply(sessionID, handleCollector.ReleaseAll(zap.L()), nil)
		}
	}()
	a.seedLeaseHolding(ctx, sessionID)

	agentCtx := &agentContext{
		Context:          ctx,
		session:          sess,
		tracer:           a.tracer,
		progressCallback: progressCallback,
		taskBinding:      taskBinding,
		turnIndex:        turnIndex,
		userMessage:      turnUserMessage,
	}

	if batch, rowless, err := locateParkedBatch(sess, nil); err == nil && len(rowless) > 0 {
		a.interruptCalls(agentCtx, sess, batch, rowless)
		span.AddEvent("continue.interrupted_calls", map[string]interface{}{"count": len(rowless)})
	}

	response, err := a.runConversationLoop(agentCtx)

	var parkedTerminal *TurnParkedError
	if !errors.As(err, &parkedTerminal) {
		defer a.completeImplicitTask(ctx, taskBinding, sessionID, int(turnIndex), implicitCloseReason(response, err), err != nil)
	}

	duration := time.Since(startTime)
	if err != nil {
		if errors.As(err, &parkedTerminal) {
			parked = true
			a.parkHandles(sessionID, handleCollector)
			span.AddEvent("conversation.parked", map[string]interface{}{
				"request_id":  parkedTerminal.RequestID,
				"duration_ms": duration.Milliseconds(),
			})
			if perr := a.memory.PersistSession(ctx, sess); perr != nil {
				zap.L().Warn("Failed to persist session at park",
					zap.String("session_id", sessionID), zap.Error(perr))
			}
			return nil, parkedTerminal
		}
		span.Status = observability.Status{Code: observability.StatusError, Message: err.Error()}
		span.AddEvent("conversation.failed", map[string]interface{}{
			"error":       err.Error(),
			"duration_ms": duration.Milliseconds(),
		})
		if progressCallback != nil {
			progressCallback(ProgressEvent{
				Stage:     StageFailed,
				Message:   fmt.Sprintf("Execution failed: %v", err),
				Timestamp: time.Now(),
			})
		}
		return nil, fmt.Errorf("conversation loop failed: %w", err)
	}

	if _, ended := response.Metadata["ended_by"]; !ended {
		a.appendMessage(ctx, sess, Message{
			Role:              "assistant",
			Content:           response.Content,
			Thinking:          response.Thinking,
			AgentID:           a.GetID(),
			Timestamp:         time.Now(),
			TokenCount:        response.Usage.TotalTokens,
			CostUSD:           response.Usage.CostUSD,
			InputTokens:       response.Usage.InputTokens,
			OutputTokens:      response.Usage.OutputTokens,
			CacheReadTokens:   response.Usage.CacheReadInputTokens,
			CacheCreateTokens: response.Usage.CacheCreationInputTokens,

			CacheCreate1hTokens: response.Usage.CacheCreation1hInputTokens,
			CacheCreate5mTokens: response.Usage.CacheCreation5mInputTokens,
			ThinkingTokens:      response.Usage.ThinkingTokens,
			Effort:              response.Usage.Effort,
		}, false)
	}
	if err := a.memory.PersistSession(ctx, sess); err != nil {
		zap.L().Warn("Failed to persist session",
			zap.String("session_id", sessionID), zap.Error(err))
		span.RecordError(err)
	}
	span.SetAttribute("conversation.duration_ms", duration.Milliseconds())
	span.Status = observability.Status{Code: observability.StatusOK}
	a.recordConversationMetrics(sessionID, response, duration)
	return response, nil
}

// unfinishedTurn reports the session's last turn — its index and opening user
// message — and whether it is unfinished: it has rows, and does not end with
// the model's final reply or a call that ended the turn.
func unfinishedTurn(sess *Session) (turnIndex int64, userMessage string, unfinished bool) {
	msgs := rawSessionMessages(sess)
	if len(msgs) == 0 {
		return 0, "", false
	}
	last := msgs[len(msgs)-1]
	turnIndex = last.Turn
	for _, m := range msgs {
		if m.Role == "user" && m.Turn == turnIndex {
			userMessage = m.Content
			break
		}
	}
	if last.Role == "assistant" && len(last.ToolCalls) == 0 {
		return turnIndex, userMessage, false
	}
	if _, rowless, err := locateParkedBatch(sess, nil); err == nil && len(rowless) == 0 && batchEndedBy(sess) != "" {
		return turnIndex, userMessage, false
	}
	return turnIndex, userMessage, true
}

// interruptCalls gives each call of the batch that has no result an
// interrupted one, in batch order, without running any tool body.
func (a *Agent) interruptCalls(ctx Context, sess *Session, batch Message, rowless []ToolCall) {
	var execs []ToolExecution
	st := &batchState{allToolExecutions: &execs, batchLen: len(batch.ToolCalls)}
	for _, call := range rowless {
		a.synthesizeParkedResult(ctx, sess, call, &shuttle.Result{
			Success: false,
			Error: &shuttle.Error{
				Code:      InterruptedCallCode,
				Message:   "this call was interrupted before it returned a result, so it may or may not have taken effect; check before calling it again",
				Retryable: true,
			},
		}, st)
	}
}
