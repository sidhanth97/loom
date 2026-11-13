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

// This file implements compacting a session on request: the in-loop fold,
// asked for between turns rather than when the context crosses its start
// mark.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/teradata-labs/loom/pkg/session"
)

// ErrNothingToCompact: the session has no history, or a fold now would remove
// nothing — its context is only the turns a fold keeps.
var ErrNothingToCompact = errors.New("nothing to compact: the context holds only its newest turns")

// Compact folds a session's context now, exactly as the in-loop fold does: the
// agent summarises the settled past on its own context, and the past is
// replaced by the summary. focus, when given, is what the person wants the
// summary to keep in view. A turn running on the session is refused with
// ErrSessionBusy, and a turn waiting on a human decision with
// SessionParkedError.
func (a *Agent) Compact(ctx context.Context, sessionID, focus string) error {
	endTurn, err := a.takeTurn(ctx, sessionID, false)
	switch {
	case errors.Is(err, errNoSessionRow):
		return ErrNothingToCompact
	case err != nil:
		return err
	}
	defer endTurn()

	ctx = session.WithSessionID(ctx, sessionID)
	unlock := a.lockSession(sessionID)
	defer unlock()

	sess := a.memory.GetOrCreateSessionWithAgent(ctx, sessionID, a.config.Name, "")
	if err := a.guardParkedTail(ctx, sessionID, sess); err != nil {
		return err
	}
	segMem, ok := sess.SegmentedMem.(*SegmentedMemory)
	if !ok || segMem == nil || !segMem.CanFold() {
		return ErrNothingToCompact
	}

	ask := a.foldAsk(ctx)
	if f := strings.TrimSpace(focus); f != "" {
		ask += "\n<system-reminder>\nThe operator asked this summary to keep in view: " + f + "\n</system-reminder>"
	}
	messages := append(segMem.SettledMessagesForLLM(), Message{Role: "user", Content: ask})
	// The tools are the ones the turns were made with: a provider refuses a
	// history of calls sent without them, and the request keeps its cache.
	tools := a.advertisedTools(sess)
	callCtx := &agentContext{Context: ctx, session: sess, tracer: a.tracer}

	// As in the loop: a reply that is not a summary is asked again once.
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := a.chatWithRetry(callCtx, messages, tools)
		if err != nil {
			return fmt.Errorf("compact: %w", err)
		}
		if len(resp.ToolCalls) > 0 {
			continue
		}
		summary, ok := parseFoldReply(resp.Content)
		if !ok {
			continue
		}
		applied, estimate := segMem.ApplyFoldReply(ctx, summary)
		if !applied {
			return fmt.Errorf("compact: the summary could not be applied")
		}
		zap.L().Info("compact: applied",
			zap.String("session_id", sessionID),
			zap.Int("estimate_tokens", estimate))
		return nil
	}
	return fmt.Errorf("compact: the model did not reply with a summary")
}
