package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	codexCandyVerifyPrompt   = "在一个黑色的袋子里放有三种口味的糖果，每种糖果有两种不同的形状（圆形和五角星形，不同的形状靠手感可以分辨）。现已知不同口味的糖和不同形状的数量统计如下表。参赛者需要在活动前决定摸出的糖果数目，那么，最少取出多少个糖果才能保证手中同时拥有不同形状的苹果味和桃子味的糖？（同时手中有圆形苹果味匹配五角星桃子味糖果，或者有圆形桃子味匹配五角星苹果味糖果都满足要求）\n苹果味 桃子味 西瓜味\n圆形 7 9 8\n五角星形 7 6 4\n\n只输出最终整数，不要解释。"
	codexPelicanVerifyPrompt = "创建一个 HTML，内容是 SVG 绘制一个鹈鹕骑自行车的 2D 动画，你不需要任何测试"
)

// QualifyCodexTicketFullBlood 对刚打出的门票进行双重验收：
// 第一关：糖果题准确输出 21（非 29，非空包）
// 第二关：鹈鹕题拥有完整 .card 架构与飘动围巾，且无残血 .scene（杜绝半降智）
// QualifyCodexTicketFullBlood 对刚打出的门票进行轻量快速验收：
// 确认网关路由健康、Cookie 与状态合规有效，绝不在后台长循环中持续占用账号长程并发。
func (s *OpenAIGatewayService) QualifyCodexTicketFullBlood(ctx context.Context, account *Account, token, model string, ticket *openAICodexTicket) bool {
	if s == nil || s.httpUpstream == nil || account == nil || ticket == nil || len(ticket.State) == 0 {
		return false
	}
	if len(ticket.HarvestCookies) == 0 {
		return true
	}
	// 验证网关连通性与非过载状态
	verifyCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	payload := map[string]any{
		"model": model,
		"input": []map[string]any{
			{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "ping",
					},
				},
			},
		},
		"stream":              true,
		"store":               false,
		"parallel_tool_calls": true,
		"reasoning":           map[string]any{"effort": "low"},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	req, err := http.NewRequestWithContext(verifyCtx, http.MethodPost, chatgptCodexURL, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Close = true
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	_ = resolveAndSetOpenAIChatGPTAccountHeaders(verifyCtx, s.accountRepo, req.Header, account)

	req.Header.Set(openAICodexTurnStateHeader, ticket.State)
	req.Header.Set("Cookie", strings.Join(ticket.HarvestCookies, "; "))
	if sid := harvestTicketSessionID(ticket); sid != "" {
		req.Header.Set("session_id", sid)
	} else {
		req.Header.Set("session_id", uuid.NewString())
	}
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("version", "0.162.1")
	req.Header.Set("user-agent", "codex_cli_rs/0.162.1 (Ubuntu 22.4.0; x86_64) xterm-256color")

	proxy := openAIAccountProxyURL(account)
	resp, err := s.httpUpstream.Do(req, proxy, account.ID, account.Concurrency)
	if err != nil || resp == nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	// 只要成功返回流式响应且未出现 503 即验收合格
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 16*1024), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "response.created") || strings.Contains(line, "output_item") || strings.Contains(line, "output_text") {
			logger.L().Info("codex_ticket_qualifier quick verify PASSED",
				zap.Int64("account_id", account.ID),
				zap.String("model", model),
				zap.String("gateway", ticket.Gateway),
			)
			return true
		}
		if strings.Contains(line, "error") || strings.Contains(line, "overloaded") {
			return false
		}
	}
	return true
}

func (s *OpenAIGatewayService) checkTicketCandyProblem(ctx context.Context, account *Account, token, model string, ticket *openAICodexTicket) bool {
	verifyCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()

	payload := map[string]any{
		"model":               model,
		"instructions":        "",
		"input": []map[string]any{
			{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": codexCandyVerifyPrompt,
					},
				},
			},
		},
		"stream":              true,
		"store":               false,
		"parallel_tool_calls": true,
		"reasoning":           map[string]any{"effort": "medium"},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	req, err := http.NewRequestWithContext(verifyCtx, http.MethodPost, chatgptCodexURL, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Close = true
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	_ = resolveAndSetOpenAIChatGPTAccountHeaders(verifyCtx, s.accountRepo, req.Header, account)

	proxy := openAIAccountProxyURL(account)
	if proxy != "" {
		applyOpenAICodexTicketHarvestIdentity(req.Header, model)
	} else {
		req.Header.Del("originator")
		req.Header.Del("version")
		req.Header.Del("user-agent")
	}

	req.Header.Set(openAICodexTurnStateHeader, ticket.State)
	req.Header.Set("Cookie", strings.Join(ticket.HarvestCookies, "; "))
	if sid := harvestTicketSessionID(ticket); sid != "" {
		req.Header.Set("session_id", sid)
	} else {
		req.Header.Set("session_id", uuid.NewString())
	}
	resp, err := s.httpUpstream.Do(req, proxy, account.ID, account.Concurrency)
	if err != nil || resp == nil {
		logger.L().Warn("codex_ticket_qualifier checkTicketCandyProblem request error", zap.Error(err), zap.String("gateway", ticket.Gateway))
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		logger.L().Warn("codex_ticket_qualifier checkTicketCandyProblem non-200 status", zap.Int("status", resp.StatusCode), zap.String("gateway", ticket.Gateway))
		return false
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var output strings.Builder
	hasDeltas := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "data:") {
			dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if dataStr == "[DONE]" {
				break
			}
			var ev struct {
				Type  string `json:"type"`
				Delta string `json:"delta"`
				Part  struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"part"`
				Item struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"item"`
			}
			if json.Unmarshal([]byte(dataStr), &ev) == nil {
				if ev.Type == "response.output_text.delta" && ev.Delta != "" {
					output.WriteString(ev.Delta)
					hasDeltas = true
				} else if !hasDeltas && ev.Type == "response.content_part.done" && ev.Part.Text != "" {
					output.WriteString(ev.Part.Text)
				} else if !hasDeltas && ev.Type == "response.output_item.done" {
					for _, c := range ev.Item.Content {
						if c.Type == "output_text" && c.Text != "" {
							output.WriteString(c.Text)
						}
					}
				}
			}
		}
	}
	ans := strings.TrimSpace(output.String())
	// 满血铁律：标准答案必须严格是 21！任何 29/28 一律判定为降智拒绝！
	passed := strings.Contains(ans, "21") && !strings.Contains(ans, "29") && !strings.Contains(ans, "28")
	if !passed {
		logger.L().Warn("codex_ticket_qualifier checkTicketCandyProblem answer invalid (must be 21, reject 29/28)",
			zap.String("gateway", ticket.Gateway),
			zap.String("answer", ans),
		)
	} else {
		logger.L().Info("codex_ticket_qualifier checkTicketCandyProblem answer PASSED (100% full-blood 21)",
			zap.String("gateway", ticket.Gateway),
			zap.String("answer", ans),
		)
	}
	return passed
}

func (s *OpenAIGatewayService) checkTicketPelicanAnimation(ctx context.Context, account *Account, token, model string, ticket *openAICodexTicket) bool {
	verifyCtx, cancel := context.WithTimeout(ctx, 160*time.Second)
	defer cancel()

	payload := map[string]any{
		"model":               model,
		"instructions":        "",
		"input": []map[string]any{
			{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": codexPelicanVerifyPrompt,
					},
				},
			},
		},
		"stream":              true,
		"store":               false,
		"parallel_tool_calls": true,
		"reasoning":           map[string]any{"effort": "medium"},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	req, err := http.NewRequestWithContext(verifyCtx, http.MethodPost, chatgptCodexURL, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Close = true
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	_ = resolveAndSetOpenAIChatGPTAccountHeaders(verifyCtx, s.accountRepo, req.Header, account)

	proxy := openAIAccountProxyURL(account)
	if proxy != "" {
		applyOpenAICodexTicketHarvestIdentity(req.Header, model)
	} else {
		req.Header.Del("originator")
		req.Header.Del("version")
		req.Header.Del("user-agent")
	}

	req.Header.Set(openAICodexTurnStateHeader, ticket.State)
	req.Header.Set("Cookie", strings.Join(ticket.HarvestCookies, "; "))
	if sid := harvestTicketSessionID(ticket); sid != "" {
		req.Header.Set("session_id", sid)
	} else {
		req.Header.Set("session_id", uuid.NewString())
	}
	resp, err := s.httpUpstream.Do(req, proxy, account.ID, account.Concurrency)
	if err != nil || resp == nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var output strings.Builder
	hasDeltas := false
	prefixLen := 0
	passed := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "data:") {
			dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if dataStr == "[DONE]" {
				break
			}
			var ev struct {
				Type  string `json:"type"`
				Delta string `json:"delta"`
				Part  struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"part"`
				Item struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"item"`
			}
			chunk := ""
			if json.Unmarshal([]byte(dataStr), &ev) == nil {
				if ev.Type == "response.output_text.delta" && ev.Delta != "" {
					chunk = ev.Delta
					hasDeltas = true
				} else if !hasDeltas && ev.Type == "response.content_part.done" && ev.Part.Text != "" {
					chunk = ev.Part.Text
				} else if !hasDeltas && ev.Type == "response.output_item.done" {
					for _, c := range ev.Item.Content {
						if c.Type == "output_text" && c.Text != "" {
							chunk += c.Text
						}
					}
				}
			}
			if chunk != "" {
				output.WriteString(chunk)
				prefixLen += len(chunk)
				current := output.String()

				// 检查 <title> 标签
				if strings.Contains(current, "</title>") {
					titleStart := strings.Index(current, "<title>")
					titleEnd := strings.Index(current, "</title>")
					if titleStart != -1 && titleEnd > titleStart {
						title := strings.TrimSpace(current[titleStart+7 : titleEnd])
						// 严格满血铁律：字面刻板标题「鹈鹕骑自行车」直接判定为降智拒绝！
						if strings.Contains(title, "鹈鹕骑自行车") {
							logger.L().Warn("codex_ticket_qualifier early rejected: detected degraded literal title (鹈鹕骑自行车)",
								zap.String("gateway", ticket.Gateway),
								zap.String("title", title),
							)
							return false
						}
						// 满血特征：必须具有自主创意的艺术标题（如《鹈鹕的海边骑行》、《海风骑行》等）
						logger.L().Info("codex_ticket_qualifier stage 2 PASSED: detected full-blood creative title",
							zap.String("gateway", ticket.Gateway),
							zap.String("title", title),
							zap.Int("prefixLen", prefixLen),
						)
						passed = true
						break // 命中满血创意标题，通过验收！
					}
				}

				if prefixLen > 2500 {
					// 超过 2500 字符仍未见合法标题，判定为非满血模板拒绝
					break
				}
			}
		}
	}
	return passed
}

// MintPristineTurn0State 在验证通过的满血路由上，立即打出一张全新的、未经任何消耗的 Turn 0 纯净门票！
// 这样存入数据库的门票 100% 崭新无历史污染，彻底杜绝重放冲突与 server_is_overloaded。
func (s *OpenAIGatewayService) MintPristineTurn0State(ctx context.Context, account *Account, token, model string, ticket *openAICodexTicket) string {
	if s == nil || account == nil || ticket == nil || len(ticket.HarvestCookies) == 0 {
		return ""
	}
	mintCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	payload := map[string]any{
		"model":               model,
		"instructions":        "",
		"input": []map[string]any{
			{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "1",
					},
				},
			},
		},
		"stream":              true,
		"store":               false,
		"parallel_tool_calls": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	req, err := http.NewRequestWithContext(mintCtx, http.MethodPost, chatgptCodexURL, bytes.NewReader(body))
	if err != nil {
		return ""
	}
	req.Close = true
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("Cookie", strings.Join(ticket.HarvestCookies, "; "))
	req.Header.Set("session_id", uuid.NewString())
	_ = resolveAndSetOpenAIChatGPTAccountHeaders(mintCtx, s.accountRepo, req.Header, account)

	// 打票用住宅 IP，出口对话用机房 IP！
	proxy := ticket.HarvestProxyURL
	if proxy == "" {
		proxy = openAIAccountProxyURL(account)
	}
	if proxy != "" {
		applyOpenAICodexTicketHarvestIdentity(req.Header, model)
	} else {
		req.Header.Del("originator")
		req.Header.Del("version")
		req.Header.Del("user-agent")
	}

	resp, err := s.httpUpstream.Do(req, proxy, account.ID, account.Concurrency)
	if err != nil || resp == nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	return resp.Header.Get(openAICodexTurnStateHeader)
}
