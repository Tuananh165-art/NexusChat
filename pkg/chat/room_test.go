package chat

import "testing"

func TestNewInviteCode(t *testing.T) {
	code, err := newInviteCode()
	if err != nil {
		t.Fatalf("newInviteCode returned error: %v", err)
	}
	if len(code) != 8 {
		t.Fatalf("expected 8-character invite code, got %q", code)
	}
	for _, r := range code {
		if r == '0' || r == '1' || r == 'I' || r == 'O' {
			t.Fatalf("invite code contains ambiguous character %q in %q", r, code)
		}
	}
}

func TestParseMemberIDs(t *testing.T) {
	got, err := parseMemberIDs([]string{"123", " 456 ", ""})
	if err != nil {
		t.Fatalf("parseMemberIDs returned error: %v", err)
	}
	if len(got) != 2 || got[0] != 123 || got[1] != 456 {
		t.Fatalf("unexpected member IDs: %#v", got)
	}
	if _, err := parseMemberIDs([]string{"abc"}); err == nil {
		t.Fatal("expected invalid member ID error")
	}
}
