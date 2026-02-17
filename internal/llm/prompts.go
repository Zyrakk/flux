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
	sb.WriteString(`Clasifica estos artículos. Para cada uno, responde con:
- article_id: el ID proporcionado
- relevant: true/false
- section: una de [cybersecurity, tech, economy, world] (confirma o corrige la sección asignada)
- clickbait: true/false
- reason: una frase explicando por qué es o no relevante

Artículos:
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
Responde SOLO con un JSON array.`)

	return sb.String()
}

// BuildSummarizePrompt creates the single-article summarization prompt.
func BuildSummarizePrompt(article ArticleInput) string {
	return fmt.Sprintf(`Resume este artículo en 2-3 frases. Si es una vulnerabilidad, incluye severidad
y si hay parche. Si es código/herramienta, explica qué hace y por qué importa.
Si hay datos concretos (benchmarks, cifras), inclúyelos.
Si es una noticia financiera, incluye cifras clave y tendencia.

Título: %s
Fuente: %s
Sección: %s

%s`, article.Title, article.SourceType, article.Section, truncateContent(article.Content, 4000))
}

// BuildBriefingPrompt creates the final briefing synthesis prompt.
func BuildBriefingPrompt(sections []BriefingSection) string {
	var sb strings.Builder
	sb.WriteString(`Genera un briefing matutino organizado en las siguientes secciones.
Para cada sección, destaca el artículo más importante primero.
Si hay artículos relacionados entre secciones, conéctalos explícitamente.
Si un artículo tiene múltiples fuentes, conserva explícitamente una línea con este formato:
"📡 Visto en: HN, r/netsec, ...".
Formato: Markdown. Tono: directo, técnico, sin relleno.

`)

	for _, sec := range sections {
		sb.WriteString(fmt.Sprintf("## %s (máx %d artículos)\n", sec.DisplayName, sec.MaxArticles))
		for i, a := range sec.Articles {
			sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n   %s\n", i+1, a.Title, a.URL, a.Summary))
			if len(a.ReportedBy) > 1 {
				sb.WriteString(fmt.Sprintf("   Reportado por: %s\n", strings.Join(a.ReportedBy, ", ")))
			}
			if len(a.SeenIn) > 1 {
				sb.WriteString(fmt.Sprintf("   📡 Visto en: %s\n", strings.Join(a.SeenIn, ", ")))
			}
			sb.WriteString(fmt.Sprintf("   Fuente principal: %s\n\n", a.SourceType))
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
