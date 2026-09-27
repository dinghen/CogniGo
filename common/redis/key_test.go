package redis

import "testing"

func TestUserFileIndexNamespaceIsolated(t *testing.T) {
	first := GenerateUserIndexName("alice", "doc.md")
	secondUser := GenerateUserIndexName("bob", "doc.md")
	secondFile := GenerateUserIndexName("alice", "other.md")
	if first == secondUser || first == secondFile {
		t.Fatalf("user/file index names collide: %q, %q, %q", first, secondUser, secondFile)
	}
	if got, want := GenerateUserIndexNamePrefix("alice", "doc.md"), "rag_docs:alice:doc.md:"; got != want {
		t.Fatalf("unexpected index prefix: got %q want %q", got, want)
	}
}
