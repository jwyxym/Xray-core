package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type configPathPart struct {
	key   string
	index int
	array bool
}

func parseConfigPath(path string) ([]configPathPart, error) {
	var parts []configPathPart
	for pos := 0; pos < len(path); {
		start := pos
		for pos < len(path) && path[pos] != '.' && path[pos] != '[' && path[pos] != ']' {
			pos++
		}
		if start == pos {
			return nil, fmt.Errorf("expected field name at offset %d", pos)
		}
		parts = append(parts, configPathPart{key: path[start:pos]})
		for pos < len(path) && path[pos] == '[' {
			pos++
			start = pos
			for pos < len(path) && path[pos] >= '0' && path[pos] <= '9' {
				pos++
			}
			if start == pos || pos == len(path) || path[pos] != ']' {
				return nil, fmt.Errorf("expected non-negative array index at offset %d", start)
			}
			index, err := strconv.Atoi(path[start:pos])
			if err != nil {
				return nil, fmt.Errorf("array index is too large")
			}
			parts = append(parts, configPathPart{index: index, array: true})
			pos++
		}
		if pos == len(path) {
			break
		}
		if path[pos] != '.' || pos+1 == len(path) {
			return nil, fmt.Errorf("expected dot followed by field name at offset %d", pos)
		}
		pos++
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty config path")
	}
	return parts, nil
}

func setConfigPath(node any, parts []configPathPart, value any) (any, error) {
	if len(parts) == 0 {
		return value, nil
	}
	part := parts[0]
	if part.array {
		var items []any
		if node != nil {
			var ok bool
			items, ok = node.([]any)
			if !ok {
				return nil, fmt.Errorf("expected array at index %d", part.index)
			}
		}
		if part.index > len(items) {
			return nil, fmt.Errorf("array index %d exceeds length %d; append at the current length", part.index, len(items))
		}
		if part.index == len(items) {
			items = append(items, nil)
		}
		updated, err := setConfigPath(items[part.index], parts[1:], value)
		if err != nil {
			return nil, err
		}
		items[part.index] = updated
		return items, nil
	}
	var fields map[string]any
	if node == nil {
		fields = make(map[string]any)
	} else {
		var ok bool
		fields, ok = node.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object at field %q", part.key)
		}
	}
	updated, err := setConfigPath(fields[part.key], parts[1:], value)
	if err != nil {
		return nil, err
	}
	fields[part.key] = updated
	return fields, nil
}

func applyConfigSets(base []byte, assignments []string) ([]byte, error) {
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(base))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode base config: %w", err)
	}
	if root == nil {
		root = make(map[string]any)
	}
	for _, assignment := range assignments {
		path, raw, ok := strings.Cut(assignment, "=")
		if !ok {
			return nil, fmt.Errorf("--set requires path=value")
		}
		parts, err := parseConfigPath(path)
		if err != nil {
			return nil, fmt.Errorf("--set path %q: %w", path, err)
		}
		var value any = raw
		if json.Valid([]byte(raw)) {
			decoder := json.NewDecoder(strings.NewReader(raw))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil {
				return nil, fmt.Errorf("--set path %q: invalid JSON value", path)
			}
		}
		if _, err := setConfigPath(root, parts, value); err != nil {
			return nil, fmt.Errorf("--set path %q: %w", path, err)
		}
	}
	return json.MarshalIndent(root, "", "\t")
}
