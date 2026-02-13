package dedup

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"basic normalization",
			"https://example.com/article",
			"https://example.com/article",
		},
		{
			"remove www",
			"https://www.example.com/article",
			"https://example.com/article",
		},
		{
			"remove trailing slash",
			"https://example.com/article/",
			"https://example.com/article",
		},
		{
			"keep root slash",
			"https://example.com/",
			"https://example.com/",
		},
		{
			"lowercase host",
			"https://EXAMPLE.COM/Article",
			"https://example.com/Article",
		},
		{
			"remove utm params",
			"https://example.com/article?utm_source=twitter&utm_medium=social&id=123",
			"https://example.com/article?id=123",
		},
		{
			"remove fbclid",
			"https://example.com/article?fbclid=abc123&page=1",
			"https://example.com/article?page=1",
		},
		{
			"remove fragment",
			"https://example.com/article#comments",
			"https://example.com/article",
		},
		{
			"sort query params",
			"https://example.com/search?z=1&a=2&m=3",
			"https://example.com/search?a=2&m=3&z=1",
		},
		{
			"combined normalization",
			"https://WWW.Example.COM/article/?utm_source=hn&utm_campaign=test&id=42#top",
			"https://example.com/article?id=42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestHashURL(t *testing.T) {
	// Same URL with different tracking params should hash identically
	hash1 := HashURL("https://example.com/article?utm_source=twitter")
	hash2 := HashURL("https://example.com/article?utm_source=reddit")
	hash3 := HashURL("https://example.com/article")
	assert.Equal(t, hash1, hash2)
	assert.Equal(t, hash2, hash3)

	// Different URLs should hash differently
	hash4 := HashURL("https://example.com/other-article")
	assert.NotEqual(t, hash1, hash4)

	// www vs non-www should hash identically
	hash5 := HashURL("https://www.example.com/article")
	assert.Equal(t, hash1, hash5)
}
