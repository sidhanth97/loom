package llm

import (
	"reflect"
	"slices"

	"github.com/teradata-labs/loom/pkg/shuttle"
)

// NormalizeObjectToolSchema adapts roots for providers requiring plain objects.
// Root alternatives advertise their fields and common requirements; the original
// tool schema remains responsible for validating correlations between fields.
func NormalizeObjectToolSchema(schema *shuttle.JSONSchema) map[string]interface{} {
	root := schema.ToToolMap()
	if root == nil {
		return nil
	}
	return normalizeObjectSchemaRoot(root)
}

func normalizeObjectSchemaRoot(root map[string]interface{}) map[string]interface{} {
	properties, _ := root["properties"].(map[string]interface{})
	if properties == nil {
		properties = make(map[string]interface{})
	}
	required, _ := root["required"].([]string)
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		branches, _ := root[keyword].([]map[string]interface{})
		branchProperties := make(map[string][]interface{})
		var branchRequired []string
		for index, branch := range branches {
			if branch == nil {
				branch = make(map[string]interface{})
			}
			branch = normalizeObjectSchemaRoot(branch)
			for name, property := range branch["properties"].(map[string]interface{}) {
				branchProperties[name] = append(branchProperties[name], property)
			}
			fields, _ := branch["required"].([]string)
			if keyword == "allOf" || index == 0 {
				branchRequired = appendUniqueRequired(branchRequired, fields)
			} else {
				branchRequired = slices.DeleteFunc(branchRequired, func(name string) bool {
					return !slices.Contains(fields, name)
				})
			}
		}
		for name, alternatives := range branchProperties {
			property := alternatives[0]
			if len(alternatives) > 1 {
				propertyKeyword := "anyOf"
				if keyword == "allOf" {
					propertyKeyword = "allOf"
				}
				property = map[string]interface{}{propertyKeyword: alternatives}
			}
			if existing, ok := properties[name]; ok && !reflect.DeepEqual(existing, property) {
				property = map[string]interface{}{"allOf": []interface{}{existing, property}}
			}
			properties[name] = property
		}
		required = appendUniqueRequired(required, branchRequired)
		delete(root, keyword)
	}
	root["type"] = "object"
	root["properties"] = properties
	if len(required) > 0 {
		root["required"] = required
	} else {
		delete(root, "required")
	}
	return root
}

func appendUniqueRequired(required, fields []string) []string {
	for _, name := range fields {
		if !slices.Contains(required, name) {
			required = append(required, name)
		}
	}
	return required
}
