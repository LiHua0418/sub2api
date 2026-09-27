package basispoints

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHostedToolOmissionIsExplicitAndDeterministic(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "get_weather", "parameters": object{"type": "object"}}, object{"type": "web_search"}, object{"type": "image_generation"}}
	body, bridge := mustPrepare(t, source, "scope", nil)
	if len(bridge.Warnings) != 1 || !strings.Contains(bridge.Warnings[0], "image_generation, web_search") {
		t.Fatalf("missing capability warning: %v", bridge.Warnings)
	}
	encoded, _ := json.Marshal(body)
	if !strings.Contains(string(encoded), "Do not claim to have used them") {
		t.Fatal("model must know omitted hosted capabilities are unavailable")
	}
	if _, present := body["tools"]; present {
		t.Fatal("native schema must not be forwarded")
	}
	source["tool_choice"] = "none"
	_, bridge = mustPrepare(t, source, "scope", nil)
	if len(bridge.tools) != 0 || len(bridge.Warnings) != 0 {
		t.Fatal("none must skip the entire tool catalog")
	}
}

func TestTurnWithoutUserRemainsStableAcrossToolResults(t *testing.T) {
	source := testSource()
	source["input"] = []any{message("developer", "synthetic task")}
	first, _ := mustPrepare(t, source, "scope", nil)
	source["input"] = append(mustTestValue[[]any](t, source["input"]), object{"type": "function_call", "call_id": "call_old", "name": "get_weather", "arguments": `{"city":"Tokyo"}`}, object{"type": "function_call_output", "call_id": "call_old", "output": "18 C"})
	next, _ := mustPrepare(t, source, "scope", nil)
	a, b := mustTestValue[object](t, first["metadata"]), mustTestValue[object](t, next["metadata"])
	if !reflect.DeepEqual(a["turn_id"], b["turn_id"]) || b["agent_iteration"] != "2" {
		t.Fatalf("unstable continuation metadata: %v / %v", a, b)
	}
}

func TestAssistantEncryptedContentInHistoryIsCleanedAndLifted(t *testing.T) {
	for _, tc := range []struct {
		name string
		item object
	}{
		{
			name: "typed message assistant",
			item: object{
				"type": "message",
				"role": "assistant",
				"content": []any{
					object{"type": "output_text", "text": "previous reply"},
					object{"type": "encrypted_content", "encrypted_content": "gAAAAABencryptedBlob"},
				},
			},
		},
		{
			name: "untyped assistant with role only",
			item: object{
				"role": "assistant",
				"content": []any{
					object{"type": "output_text", "text": "previous reply without type"},
					object{"type": "encrypted_content", "encrypted_content": "gAAAAABencryptedBlob"},
				},
			},
		},
		{
			name: "assistant with data field in encrypted content",
			item: object{
				"role": "assistant",
				"content": []any{
					object{"type": "output_text", "text": "previous reply with data"},
					object{"type": "encrypted_content", "data": "gAAAAABencryptedBlob"},
				},
			},
		},
		{
			name: "assistant with empty encrypted content",
			item: object{
				"role": "assistant",
				"content": []any{
					object{"type": "output_text", "text": "previous reply with empty enc"},
					object{"type": "encrypted_content"},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := testSource()
			source["input"] = []any{
				message("user", "first question"),
				tc.item,
				message("user", "second question"),
			}
			raw, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			out, bridge, err := Prepare(raw, "scope", nil)
			if err != nil {
				t.Fatalf("Prepare must succeed and strip encrypted_content from assistant history: %v", err)
			}
			if bridge == nil {
				t.Fatal("expected non-nil bridge")
			}
			outJSON, _ := json.Marshal(out)
			if strings.Contains(string(outJSON), "type=encrypted_content") || strings.Contains(string(outJSON), "\"type\":\"encrypted_content\"") {
				t.Fatalf("prepared wire request must not contain encrypted_content part in message content: %s", string(outJSON))
			}
		})
	}
}
