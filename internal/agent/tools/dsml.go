package tools

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// DSML 文本工具调用标记的解析支持。
//
// 部分模型（如 deepseek-flash 开启 thinking 后）不走原生 tool_calls 通道，
// 而是把工具调用以 DSML 标记写进 content：
//
//	<｜｜DSML｜｜ calls>
//	<｜｜DSML｜｜ invoke name="wiki_search">
//	<｜｜DSML｜｜ parameter name="query" string="true">关键词</｜｜DSML｜｜ parameter>
//	<｜｜DSML｜｜ parameter name="limit" string="false">10</｜｜DSML｜｜ parameter>
//	</｜｜DSML｜｜ invoke>
//	</｜｜DSML｜｜ calls>
//
// 若不解析，这段原文会被当成最终答案直接回给用户（observe.go 的
// length/natural-stop finalize 只看 len(ToolCalls)==0）。
var (
	// dsmlInvokeOpenRe 匹配 invoke 开标签，捕获工具名。
	dsmlInvokeOpenRe = regexp.MustCompile(`<[｜|]{1,2}DSML[｜|]{1,2}\s*invoke\s+name="([^"]+)"\s*>`)
	// dsmlInvokeCloseRe 匹配 invoke 闭标签。
	dsmlInvokeCloseRe = regexp.MustCompile(`</[｜|]{1,2}DSML[｜|]{1,2}\s*invoke\s*>`)
	// dsmlParamRe 匹配完整 parameter 块，捕获参数名、string 标志与参数值。
	// (?s) 让参数值可以跨行。
	dsmlParamRe = regexp.MustCompile(`(?s)<[｜|]{1,2}DSML[｜|]{1,2}\s*parameter\s+name="([^"]+)"(?:\s+string="([^"]*)")?\s*>(.*?)</[｜|]{1,2}DSML[｜|]{1,2}\s*parameter\s*>`)
	// dsmlLeftoverTagRe 匹配残留的任意 DSML 标签（calls 开闭、孤立标记等）。
	dsmlLeftoverTagRe = regexp.MustCompile(`</?[｜|]{1,2}DSML[｜|]{1,2}[^>]*>`)
)

// RecoverDSMLToolCalls 从 content 中恢复以 DSML 文本标记书写的工具调用。
// 返回恢复出的调用列表与剔除标记后的剩余文本；没有可恢复的调用时返回
// (nil, content)，即原文保持不变。
//
// 容忍的变体：标记前的零宽空格（U+200B）、ASCII 半角竖线（| 与 ||）、
// calls 包裹缺失/错乱、多个 invoke 连续出现、缺失闭合标签（截断）。
// string="true" 的参数值按字面字符串处理，string="false" 的参数值若是
// 合法 JSON 则按 JSON 值处理（如数组、数字、布尔），否则回退为字符串。
func RecoverDSMLToolCalls(content string) ([]types.LLMToolCall, string) {
	if content == "" || !strings.Contains(content, "DSML") {
		return nil, content
	}
	// 零宽空格只影响标记识别，先剥掉让正则按统一形态匹配。
	stripped := strings.ReplaceAll(content, "\u200b", "")

	opens := dsmlInvokeOpenRe.FindAllStringSubmatchIndex(stripped, -1)
	if len(opens) == 0 {
		return nil, content
	}

	var calls []types.LLMToolCall
	var kept []byte
	cursor := 0

	for i, m := range opens {
		start, nameStart, nameEnd := m[0], m[2], m[3]
		name := stripped[nameStart:nameEnd]

		// 闭标签搜索截止到下一个 invoke 开标签；找不到闭标签时（截断）
		// 区间延到下一个开标签或文本末尾。end 不会超过下一个 start，
		// 各区间天然按顺序不重叠。
		searchFrom, searchTo := m[1], len(stripped)
		if i+1 < len(opens) {
			searchTo = opens[i+1][0]
		}
		end := searchTo
		if loc := dsmlInvokeCloseRe.FindStringIndex(stripped[searchFrom:searchTo]); loc != nil {
			end = searchFrom + loc[1]
		}

		kept = append(kept, stripped[cursor:start]...)
		cursor = end

		calls = append(calls, types.LLMToolCall{
			ID:       NormalizeToolCallID("", name, len(calls)),
			Type:     "function",
			Function: types.FunctionCall{Name: name, Arguments: dsmlArgsJSON(stripped[start:end])},
		})
	}
	kept = append(kept, stripped[cursor:]...)

	if len(calls) == 0 {
		return nil, content
	}

	rest := dsmlLeftoverTagRe.ReplaceAllString(string(kept), "")
	return calls, trimWhitespace(rest)
}

// dsmlArgsJSON 提取 invoke 块内全部 parameter 并编码为工具入参 JSON。
// 无参数或编码失败时返回 "{}"（空对象可被正常 unmarshal 执行）。
func dsmlArgsJSON(block string) string {
	params := dsmlParamRe.FindAllStringSubmatch(block, -1)
	if len(params) == 0 {
		return "{}"
	}
	args := make(map[string]any, len(params))
	for _, p := range params {
		name, strFlag, value := p[1], p[2], p[3]
		// string="false" 表示值是结构化数据；解析为 JSON 保留数组/数字等类型。
		if strings.EqualFold(strFlag, "false") {
			raw := strings.TrimSpace(value)
			if json.Valid([]byte(raw)) {
				args[name] = json.RawMessage(raw)
				continue
			}
		}
		args[name] = value
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
