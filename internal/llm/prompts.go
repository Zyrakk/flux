package llm

import (
	"fmt"
	"strings"
)

// Prompt templates for the LLM pipeline.

const systemPrompt = `You are Flux, an intelligent news analysis system. You are precise, technical, and concise. You never add filler or unnecessary commentary.`

// BuildClassifyPrompt creates the batch classification prompt.
func BuildClassifyPrompt(articles []ArticleInput) string {
	var sb strings.Builder
	sb.WriteString(`Classify these articles. For each one, respond with:
- article_id: the provided ID
- relevant: true/false
- section: one of [cybersecurity, tech, economy, world] (confirm or correct the assigned section)
- clickbait: true/false
- reason: one sentence explaining why it is or is not relevant

Articles:
`)

	for i, a := range articles {
		content := a.Content
		if len(content) > 200 {
			content = content[:200]
		}
		sb.WriteString(fmt.Sprintf("%d. [ID: %s] %s - %s - %s\n",
			i+1, a.ID, a.Title, a.Section, content))
	}

	sb.WriteString(`
Respond ONLY with a JSON array.`)

	return sb.String()
}

// BuildSummarizePrompt creates the single-article summarization prompt.
func BuildSummarizePrompt(article ArticleInput) string {
	return fmt.Sprintf(`Summarize this article in 2-3 sentences. If it's a vulnerability, include severity
and whether a patch exists. If it's code/tool, explain what it does and why it matters.
If there are concrete data points (benchmarks, figures), include them.
If it's financial news, include key figures and trend.

Title: %s
Source: %s
Section: %s

%s`, article.Title, article.SourceType, article.Section, truncateContent(article.Content, 4000))
}

// BuildBriefingPrompt creates the final briefing synthesis prompt.
func BuildBriefingPrompt(sections []BriefingSection) string {
	var sb strings.Builder
	sb.WriteString(`Generate a morning briefing organized into the following sections.
For each section, highlight the most important article first.
If there are related articles across sections, connect them explicitly.
If an article has multiple sources, explicitly keep a line with this format:
"📡 Seen in: HN, r/netsec, ...".
Format: Markdown. Tone: direct, technical, no filler.

`)

	for _, sec := range sections {
		sb.WriteString(fmt.Sprintf("## %s (max %d articles)\n", sec.DisplayName, sec.MaxArticles))
		for i, a := range sec.Articles {
			sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n   %s\n", i+1, a.Title, a.URL, a.Summary))
			if len(a.ReportedBy) > 1 {
				sb.WriteString(fmt.Sprintf("   Reported by: %s\n", strings.Join(a.ReportedBy, ", ")))
			}
			if len(a.SeenIn) > 1 {
				sb.WriteString(fmt.Sprintf("   📡 Seen in: %s\n", strings.Join(a.SeenIn, ", ")))
			}
			sb.WriteString(fmt.Sprintf("   Primary source: %s\n\n", a.SourceType))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func truncateContent(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars] + "\n[...truncated]"
}
