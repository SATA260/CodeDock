package tools

import (
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
)

func schemaOf[T any]() json.RawMessage {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		return json.RawMessage(`{"type":"object"}`)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return json.RawMessage(`{"type":"object"}`)
	}
	return raw
}
