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

package bedrock

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	llmtypes "github.com/teradata-labs/loom/pkg/llm/types"
)

// converseBody captures the Converse request a Client sends; the probe fails
// every request so only the payload matters.
func converseBody(t *testing.T, modelID string) string {
	t.Helper()
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"probe"}`))
	}))
	t.Cleanup(server.Close)

	c := &Client{
		client: bedrockruntime.NewFromConfig(aws.Config{
			Region:           "us-west-2",
			Credentials:      credentials.NewStaticCredentialsProvider("ak", "sk", ""),
			BaseEndpoint:     aws.String(server.URL),
			RetryMaxAttempts: 1,
		}),
		modelID:     modelID,
		maxTokens:   1024,
		temperature: 0.7,
		toolNameMap: map[string]string{},
		useConverse: true,
	}
	_, err := c.Chat(context.Background(), []llmtypes.Message{{Role: "user", Content: "hi"}}, nil)
	require.Error(t, err, "the probe always fails")
	require.NotEmpty(t, body, "no request reached Bedrock")
	return body
}

// OpenAI reasoning models on Bedrock reject the temperature field outright
// ("This model doesn't support the temperature field"), so it must be absent
// from the payload rather than defaulted. Other families keep it.
func TestConverse_OmitsTemperatureForOpenAI(t *testing.T) {
	for _, tt := range []struct {
		model string
		want  bool
	}{
		{"us.openai.gpt-6.1-sol", false},
		{"us.openai.gpt-6-luna", false},
		{"us.deepseek.r1-v1:0", true},
		{"qwen.qwen3-235b-a22b-v1:0", true},
	} {
		t.Run(tt.model, func(t *testing.T) {
			body := converseBody(t, tt.model)
			assert.Contains(t, body, "maxTokens", "inferenceConfig must still be sent")
			if tt.want {
				assert.Contains(t, body, "temperature")
			} else {
				assert.NotContains(t, body, "temperature")
			}
		})
	}
}

func TestSupportsTemperature(t *testing.T) {
	for model, want := range map[string]bool{
		"us.openai.gpt-6.1-sol":          false,
		"openai.gpt-oss-120b-1:0":        false,
		"us.anthropic.claude-sonnet-5-5": true,
		"us.deepseek.r1-v1:0":            true,
		"":                               true,
	} {
		assert.Equal(t, want, SupportsTemperature(model), model)
	}
}
