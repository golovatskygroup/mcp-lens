package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

var schemaCache sync.Map // key -> *jsonschema.Schema

func schemaCacheKey(toolName string, schema json.RawMessage) string {
	sum := sha256.Sum256(schema)
	return toolName + ":" + hex.EncodeToString(sum[:])
}

func compileSchema(toolName string, schema json.RawMessage) (*jsonschema.Schema, error) {
	key := schemaCacheKey(toolName, schema)
	if v, ok := schemaCache.Load(key); ok {
		return v.(*jsonschema.Schema), nil
	}
	s, err := jsonschema.CompileString(toolName+".json", string(schema))
	if err != nil {
		return nil, err
	}
	schemaCache.Store(key, s)
	return s, nil
}

func firstLeafValidationError(err *jsonschema.ValidationError) *jsonschema.ValidationError {
	if err == nil {
		return nil
	}
	if len(err.Causes) == 0 {
		return err
	}
	for _, c := range err.Causes {
		if leaf := firstLeafValidationError(c); leaf != nil {
			return leaf
		}
	}
	return err
}

func validateArgsAgainstSchema(toolName string, schema json.RawMessage, args any) error {
	if len(schema) == 0 {
		return nil
	}
	s, err := compileSchema(toolName, schema)
	if err != nil {
		return fmt.Errorf("invalid tool inputSchema for %s: %w", toolName, err)
	}
	if err := s.Validate(args); err != nil {
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			leaf := firstLeafValidationError(ve)
			loc := leaf.InstanceLocation
			if loc == "" {
				loc = "/"
			}
			msg := leaf.Message
			if msg == "" {
				msg = leaf.Error()
			}
			return fmt.Errorf("args schema validation failed for %s at %s: %s", toolName, loc, msg)
		}
		return fmt.Errorf("args schema validation failed for %s: %v", toolName, err)
	}
	return nil
}
