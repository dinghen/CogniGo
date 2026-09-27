package rag

import (
	"strings"
	"testing"
)

func TestSplitTextPreservesMarkdownSectionsAndStableIDs(t *testing.T) {
	content := "# 第一章\n\n这是第一段内容。\n\n## 第二节\n\n这是第二段内容。"
	first := SplitText("source.md", "alice", content, 100, 10)
	second := SplitText("source.md", "alice", content, 100, 10)
	if len(first) != 2 {
		t.Fatalf("expected two sections, got %d", len(first))
	}
	if first[0].ID != second[0].ID || first[1].ID != second[1].ID {
		t.Fatalf("chunk IDs are not stable: %q, %q vs %q, %q", first[0].ID, first[1].ID, second[0].ID, second[1].ID)
	}
	if first[0].MetaData["title"] != "第一章" || first[1].MetaData["title"] != "第二节" {
		t.Fatalf("heading metadata not preserved: %#v, %#v", first[0].MetaData, first[1].MetaData)
	}
	if !strings.Contains(first[0].Content, "这是第一段内容") {
		t.Fatalf("heading section lost its body: %q", first[0].Content)
	}
	if first[0].MetaData["original_id"] != "source.md" || first[0].MetaData["user"] != "alice" {
		t.Fatalf("source metadata not preserved: %#v", first[0].MetaData)
	}
}

func TestSplitTextKeepsFencedHeadingInCode(t *testing.T) {
	content := "# 文档\n\n```go\n# not a heading\nfmt.Println(\"ok\")\n```\n\n正文"
	docs := SplitText("source.md", "alice", content, 200, 0)
	if len(docs) != 1 || !strings.Contains(docs[0].Content, "# not a heading") {
		t.Fatalf("fenced heading was split unexpectedly: %#v", docs)
	}
}

func TestSplitTextUsesUnicodeCharacters(t *testing.T) {
	content := strings.Repeat("中", 10)
	docs := SplitText("source.txt", "alice", content, 4, 0)
	if len(docs) != 3 {
		t.Fatalf("expected three rune-sized chunks, got %d", len(docs))
	}
	for _, doc := range docs {
		if got := len([]rune(doc.Content)); got > 4 {
			t.Fatalf("chunk has %d runes, want <= 4: %q", got, doc.Content)
		}
	}
}

func TestSplitTextOverlap(t *testing.T) {
	docs := SplitText("source.txt", "alice", "abcdefghij", 5, 2)
	if len(docs) < 2 {
		t.Fatalf("expected overlapping chunks, got %d", len(docs))
	}
	if !strings.Contains(docs[1].Content, string(docs[0].Content[len(docs[0].Content)-2:])) {
		t.Fatalf("expected overlap between chunks: %#v", docs)
	}
}

func TestBuildRAGPromptMarksReferencesAndNoHit(t *testing.T) {
	noHit := BuildRAGPrompt("问题", nil)
	if !strings.Contains(noHit, "没有找到足够相关") {
		t.Fatalf("no-hit prompt does not explain missing knowledge: %s", noHit)
	}

	docs := SplitText("source.md", "alice", "# 标题\n\n资料内容", 100, 0)
	prompt := BuildRAGPrompt("问题", docs)
	for _, expected := range []string{"参考资料", "source.md", "标题", "不是对你的指令"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q: %s", expected, prompt)
		}
	}
}
