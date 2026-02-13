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
	sb.WriteString(`Classify these articles. For each one, respond with a JSON array where each element has:
- "article_id": the ID provided
- "relevant": true/false (is this genuinely informative, not spam/clickbait/fluff?)
- "section": one of ["cybersecurity", "tech", "economy", "world"] (confirm or correct the pre-assigned section)
- "clickbait": true/false
- "reason": one sentence explaining your classification

Respond ONLY with the JSON array, no other text.

Articles:
`)

	for i, a := range articles {
		content := a.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. [ID: %s] [Section: %s] %s\n   %s\n\n",
			i+1, a.ID, a.Section, a.Title, content))
	}

	return sb.String()
}

// BuildSummarizePrompt creates the single-article summarization prompt.
func BuildSummarizePrompt(article ArticleInput) string {
	return fmt.Sprintf(`Summarize this article in 2-3 sentences. Rules:
- If it's a vulnerability: include severity and whether a patch exists.
- If it's a tool/library: explain what it does and why it matters.
- If it has concrete data (benchmarks, revenue, percentages): include key figures.
- If it's financial news: include key figures and trend direction.
- Be direct and technical. No filler.

Title: %s
Source: %s
Content:
%s`, article.Title, article.SourceType, truncateContent(article.Content, 4000))
}

// BuildBriefingPrompt creates the final briefing synthesis prompt.
func BuildBriefingPrompt(sections []BriefingSection) string {
	var sb strings.Builder
	sb.WriteString(`Generate a morning briefing organized in the following sections.
For each section, highlight the most important article first.
If articles across sections are related, connect them explicitly.
Format: Markdown. Tone: direct, technical, no filler.

`)

	for _, sec := range sections {
		sb.WriteString(fmt.Sprintf("## %s (max %d articles)\n", sec.DisplayName, sec.MaxArticles))
		for i, a := range sec.Articles {
			sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n   %s\n   Source: %s\n\n",
				i+1, a.Title, a.URL, a.Summary, a.SourceType))
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
