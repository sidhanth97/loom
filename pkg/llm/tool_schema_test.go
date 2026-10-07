package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/teradata-labs/loom/pkg/shuttle"
)

func TestNormalizeObjectToolSchema(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty root",
			input: `{}`,
			want:  `{"type":"object","properties":{}}`,
		},
		{
			name: "allOf combines overlapping branch constraints",
			input: `{"allOf":[
				{"properties":{"idx":{"type":"integer","minimum":1}},"required":["idx"]},
				{"properties":{"idx":{"type":"integer","maximum":5}},"required":["idx"]}
			]}`,
			want: `{"type":"object","properties":{"idx":{"allOf":[
				{"type":"integer","minimum":1},{"type":"integer","maximum":5}
			]}},"required":["idx"]}`,
		},
		{
			name: "allOf preserves properties and unions required",
			input: `{"description":"Input","properties":{"idx":{"type":"integer"}},"required":["idx"],"allOf":[
				{"type":"object","properties":{"idx":{"type":"integer","minimum":1},"name":{"type":"string","description":"Name"}},"required":["idx","name"]}
			]}`,
			want: `{"type":"object","description":"Input","properties":{
				"idx":{"allOf":[{"type":"integer"},{"type":"integer","minimum":1}]},
				"name":{"type":"string","description":"Name"}
			},"required":["idx","name"]}`,
		},
		{
			name: "anyOf advertises all fields and only common requirements",
			input: `{"anyOf":[
				{"type":"object","properties":{"kind":{"type":"string","enum":["file"]},"path":{"type":"string","description":"Path"}},"required":["kind","path"]},
				{"type":"object","properties":{"kind":{"type":"string","enum":["text"]},"text":{"type":"string","description":"Text"}},"required":["kind","text"]}
			]}`,
			want: `{"type":"object","properties":{
				"kind":{"anyOf":[{"type":"string","enum":["file"]},{"type":"string","enum":["text"]}]},
				"path":{"type":"string","description":"Path"},"text":{"type":"string","description":"Text"}
			},"required":["kind"]}`,
		},
		{
			name: "oneOf leaves alternative-specific fields optional",
			input: `{"oneOf":[
				{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]},
				{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}
			]}`,
			want: `{"type":"object","properties":{"path":{"type":"string"},"text":{"type":"string"}}}`,
		},
		{
			name: "root nesting is normalized but property composites survive",
			input: `{"allOf":[{"anyOf":[
				{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"idx":{"anyOf":[{"type":"integer"},{"type":"null"}]}},"required":["idx"]}}},"required":["tasks"]},
				{"type":"object","properties":{}}
			]}]}`,
			want: `{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"idx":{"anyOf":[{"type":"integer"},{"type":"null"}]}},"required":["idx"]}}}}`,
		},
		{
			name:  "nil branch does not panic or make fields required",
			input: `{"anyOf":[null,{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}]}`,
			want:  `{"type":"object","properties":{"name":{"type":"string"}}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := shuttle.FromJSON([]byte(test.input))
			require.NoError(t, err)
			before, err := json.Marshal(schema)
			require.NoError(t, err)
			result := NormalizeObjectToolSchema(schema)
			data, err := json.Marshal(result)
			require.NoError(t, err)
			assert.JSONEq(t, test.want, string(data))
			after, err := json.Marshal(schema)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after))
		})
	}
	assert.Nil(t, NormalizeObjectToolSchema(nil))
}
