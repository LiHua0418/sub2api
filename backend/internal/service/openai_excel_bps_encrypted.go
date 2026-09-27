package service

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

// Only an explicit invalid-ciphertext rejection permits this recovery. Ordinary
// 400s, authentication, quota and transport errors must not replay a BPS request.
func isExcelBPSInvalidEncryptedContent(raw []byte) bool {
	if !gjson.ValidBytes(raw) {
		return false
	}
	if code := extractUpstreamErrorCode(raw); code != "" {
		return code == "invalid_encrypted_content"
	}
	// Accept the same diagnostic when a provider omits its error code. Do not classify
	// arbitrary mentions of encryption (or echoed request text) as this error.
	message := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(raw)))
	return strings.HasPrefix(message, "the encrypted content ") &&
		strings.Contains(message, "could not be verified") &&
		strings.Contains(message, "could not be decrypted or parsed")
}

// Prepare a single same-route retry from the already translated wire request.
// Drop only opaque reasoning, preserving messages, tools/results, attachments and
// routing metadata without decoding numbers or string contents. Compaction and
// encrypted message bodies may be the only copy of user context: never discard
// them to make a request pass.
func prepareExcelBPSInvalidEncryptedRetry(body, rejection []byte) ([]byte, bool) {
	if !isExcelBPSInvalidEncryptedContent(rejection) {
		return body, false
	}
	var request map[string]json.RawMessage
	if json.Unmarshal(body, &request) != nil {
		return body, false
	}
	var input []json.RawMessage
	if json.Unmarshal(request["input"], &input) != nil {
		return body, false
	}
	kept := make([]json.RawMessage, 0, len(input))
	removed := false
	hasHistory := false
	for _, item := range input {
		typeName := gjson.GetBytes(item, "type").String()
		encrypted := gjson.GetBytes(item, "encrypted_content")
		if typeName == "reasoning" && encrypted.Type == gjson.String && encrypted.String() != "" {
			removed = true
			continue
		}
		if encrypted.Exists() || len(gjson.GetBytes(item, "encrypted_function_args").Array()) > 0 {
			return body, false
		}
		for _, field := range []string{"content", "output"} {
			for _, part := range gjson.GetBytes(item, field).Array() {
				if part.Get("type").String() == "encrypted_content" || part.Get("encrypted_content").Exists() {
					return body, false
				}
			}
		}
		// Injected developer instructions and a compaction trigger alone are
		// not enough context to regenerate a user's task.
		role := gjson.GetBytes(item, "role").String()
		if role == "user" || role == "assistant" || typeName == "agent_message" ||
			typeName == "function_call" || typeName == "function_call_output" {
			hasHistory = true
		}
		kept = append(kept, item)
	}
	if !removed || !hasHistory {
		return body, false
	}
	var err error
	request["input"], err = json.Marshal(kept)
	if err != nil {
		return body, false
	}
	retry, err := json.Marshal(request)
	if err != nil {
		return body, false
	}
	return retry, true
}

// sanitizeExcelBPSEncryptedContent removes encrypted_content message parts and tool output parts
// across all input items so BPS history preparation never encounters invalid ciphertext.
func sanitizeExcelBPSEncryptedContent(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.GetBytes(body, "input").Exists() {
		return body, false, nil
	}
	var decoded map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &decoded); err != nil {
		return body, false, err
	}
	inputVal, hasInput := decoded["input"]
	if !hasInput {
		return body, false, nil
	}
	inputList, ok := inputVal.([]any)
	if !ok {
		return body, false, nil
	}
	changed := false
	newInput := make([]any, 0, len(inputList))
	for _, rawItem := range inputList {
		item, ok := rawItem.(map[string]any)
		if !ok {
			newInput = append(newInput, rawItem)
			continue
		}
		role, _ := item["role"].(string)
		itemType, _ := item["type"].(string)
		isAssistant := role == "assistant" || role == "model" || itemType == "assistant"

		for _, field := range []string{"content", "output"} {
			val, exists := item[field]
			if !exists {
				continue
			}
			parts, ok := val.([]any)
			if !ok {
				continue
			}
			cleanParts := make([]any, 0, len(parts))
			partChanged := false
			for _, p := range parts {
				part, ok := p.(map[string]any)
				if !ok {
					cleanParts = append(cleanParts, p)
					continue
				}
				pType, _ := part["type"].(string)
				_, hasEnc := part["encrypted_content"]
				if pType == "encrypted_content" || hasEnc {
					partChanged = true
					changed = true
					if isAssistant {
						enc, _ := part["encrypted_content"].(string)
						if enc == "" {
							enc, _ = part["data"].(string)
						}
						if enc == "" {
							enc, _ = part["content"].(string)
						}
						if enc != "" {
							newInput = append(newInput, map[string]any{
								"type":              "reasoning",
								"summary":           []any{},
								"encrypted_content": enc,
							})
						}
					}
					continue
				}
				cleanParts = append(cleanParts, p)
			}
			if partChanged {
				if len(cleanParts) == 0 && field == "content" {
					textType := "input_text"
					if isAssistant {
						textType = "output_text"
					}
					cleanParts = []any{map[string]any{"type": textType, "text": ""}}
				}
				item[field] = cleanParts
			}
		}
		newInput = append(newInput, item)
	}
	if !changed {
		return body, false, nil
	}
	decoded["input"] = newInput
	out, err := marshalOpenAIUpstreamJSON(decoded)
	if err != nil {
		return body, false, err
	}
	return out, true, nil
}

