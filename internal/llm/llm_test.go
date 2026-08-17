package llm

import "testing"

func TestExtractJSON(t *testing.T) {
	t.Parallel()

	got := ExtractJSON("```json\n{\"skills\":[\"a\"]}\n```")
	if got != `{"skills":["a"]}` {
		t.Fatalf("got %q", got)
	}
}
