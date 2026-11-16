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
package shuttle

import (
	"encoding/json"
	"reflect"
	"testing"
)

// A schema read from JSON reaches the provider with its array bounds and its
// additionalProperties, at every depth: a free-form argument object stays
// free-form, and a closed one stays closed.
func TestJSONSchema_KeepsArrayBoundsAndAdditionalProperties(t *testing.T) {
	schema, err := FromJSON([]byte(`{"type":"object","properties":{
		"questions":{"type":"array","minItems":1,"maxItems":4,"items":{"type":"object","additionalProperties":false,"properties":{"q":{"type":"string"}}}},
		"args":{"type":"object","additionalProperties":true},
		"typed":{"type":"object","additionalProperties":{"type":"string"}}
	}}`))
	if err != nil {
		t.Fatal(err)
	}
	sent, err := json.Marshal(schema.ToToolMap())
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(sent, &got); err != nil {
		t.Fatal(err)
	}
	props := got["properties"].(map[string]interface{})
	questions := props["questions"].(map[string]interface{})
	if questions["minItems"] != float64(1) || questions["maxItems"] != float64(4) {
		t.Errorf("array bounds lost: %v", questions)
	}
	if items := questions["items"].(map[string]interface{}); items["additionalProperties"] != false {
		t.Errorf("a closed nested object was opened: %v", items)
	}
	if args := props["args"].(map[string]interface{}); args["additionalProperties"] != true {
		t.Errorf("a free-form object lost additionalProperties: %v", args)
	}
	want := map[string]interface{}{"type": "string"}
	if typed := props["typed"].(map[string]interface{}); !reflect.DeepEqual(typed["additionalProperties"], want) {
		t.Errorf("a schema-valued additionalProperties changed: %v", typed["additionalProperties"])
	}
}
