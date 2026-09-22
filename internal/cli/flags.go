package cli

import (
	"encoding/json"
	"strconv"
	"strings"
)

func fieldsToBody(command string, fields map[string][]string) (map[string]json.RawMessage, *rejection) {
	spec, ok := specByCommand(command)
	if !ok {
		return nil, reject("invalid_command", 2)
	}
	allowed := map[string]bool{}
	for _, flag := range spec.flags {
		allowed[flag] = true
	}
	for name := range fields {
		if !allowed[name] {
			return nil, reject("invalid_flag", 2)
		}
	}
	payload := map[string]any{}
	switch command {
	case "observe":
		putString(payload, fields, "--text", "text")
		ref, rej := referenceFromFlags(fields)
		if rej != nil {
			return nil, rej
		}
		if ref != nil {
			payload["reference"] = ref
		}
		interval, rej := intervalFromFlags(fields)
		if rej != nil {
			return nil, rej
		}
		if interval != nil {
			payload["interval"] = interval
		}
	case "believe":
		putString(payload, fields, "--text", "text")
		if _, ok := fields["--reason-kind"]; ok {
			kind := fields["--reason-kind"][0]
			reason := map[string]any{"kind": kind}
			if ids := fields["--reason-id"]; len(ids) > 0 {
				reason["id"] = ids[0]
			}
			if texts := fields["--reason-text"]; len(texts) > 0 {
				reason["text"] = texts[0]
			}
			payload["reason"] = reason
		}
		if about := fields["--about"]; len(about) > 0 {
			payload["about"] = about
		}
		interval, rej := intervalFromFlags(fields)
		if rej != nil {
			return nil, rej
		}
		if interval != nil {
			payload["interval"] = interval
		}
	case "relate":
		putString(payload, fields, "--kind", "kind")
		putString(payload, fields, "--from", "from")
		putString(payload, fields, "--to", "to")
	case "search":
		putString(payload, fields, "--query", "query")
		if limits := fields["--limit"]; len(limits) > 0 {
			number, rej := flagInt(limits[0])
			if rej != nil {
				return nil, rej
			}
			payload["limit"] = number
		}
	case "domain-attach":
		putString(payload, fields, "--id", "id")
		putString(payload, fields, "--domain", "domain")
	default:
		if len(fields) != 0 {
			return nil, reject("invalid_flag", 2)
		}
	}
	return marshalFields(payload)
}

func referenceFromFlags(fields map[string][]string) (any, *rejection) {
	_, hasSource := fields["--source"]
	_, hasStart := fields["--start"]
	_, hasEnd := fields["--end"]
	if !hasSource && !hasStart && !hasEnd {
		return nil, nil
	}
	ref := map[string]any{}
	if hasSource {
		ref["source"] = fields["--source"][0]
	}
	if hasStart {
		number, rej := flagInt(fields["--start"][0])
		if rej != nil {
			return nil, rej
		}
		ref["start"] = number
	}
	if hasEnd {
		number, rej := flagInt(fields["--end"][0])
		if rej != nil {
			return nil, rej
		}
		ref["end"] = number
	}
	return ref, nil
}

func intervalFromFlags(fields map[string][]string) (any, *rejection) {
	_, hasStart := fields["--interval-start"]
	_, hasEnd := fields["--interval-end"]
	if !hasStart && !hasEnd {
		return nil, nil
	}
	if !hasStart || !hasEnd {
		return nil, reject("missing_field", 2)
	}
	return map[string]string{"start": fields["--interval-start"][0], "end": fields["--interval-end"][0]}, nil
}

func putString(payload map[string]any, fields map[string][]string, flag, key string) {
	if values := fields[flag]; len(values) > 0 {
		payload[key] = values[0]
	}
}

func flagInt(raw string) (int, *rejection) {
	if strings.ContainsAny(raw, ".eE") || raw == "" {
		return 0, reject("invalid_type", 2)
	}
	number, err := strconv.Atoi(raw)
	if err != nil {
		return 0, reject("invalid_type", 2)
	}
	return number, nil
}

func marshalFields(payload map[string]any) (map[string]json.RawMessage, *rejection) {
	out := map[string]json.RawMessage{}
	for key, value := range payload {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, reject("invalid_type", 2)
		}
		out[key] = raw
	}
	return out, nil
}

func specByCommand(command string) (commandSpec, bool) {
	name := helpTopicFor(command)
	for _, spec := range commandRegistry() {
		if spec.name == name {
			return spec, true
		}
	}
	return commandSpec{}, false
}

func helpTopicFor(command string) string {
	switch command {
	case "believe":
		return "belief add"
	case "relate":
		return "relation add"
	case "domain-attach":
		return "domain attach"
	case "reindex":
		return "embed reindex"
	default:
		return command
	}
}

func schemaDocument() map[string]any {
	fields := map[string][]string{
		"init":          []string{},
		"observe":       {"text", "reference", "interval"},
		"belief add":    {"text", "reason", "about", "interval"},
		"relation add":  {"kind", "from", "to"},
		"search":        {"query", "limit"},
		"domain attach": {"id", "domain"},
		"embed reindex": []string{},
	}
	var commands []map[string]any
	for _, spec := range commandRegistry() {
		if !spec.json {
			continue
		}
		commands = append(commands, map[string]any{"name": spec.name, "fields": fields[spec.name]})
	}
	return map[string]any{"commands": commands}
}
