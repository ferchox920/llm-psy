package service

import (
	"testing"

	"github.com/google/uuid"

	"clone-llm/internal/domain"
)

func TestDetectActiveCharacters_TrimsAndSkipsEmptyNames(t *testing.T) {
	chars := []domain.Character{
		{ID: uuid.New(), Name: "  Ana  "},
		{ID: uuid.New(), Name: ""},
		{ID: uuid.New(), Name: "Luis"},
	}

	got := detectActiveCharacters(chars, "  Hable con ana ayer  ")
	if len(got) != 1 {
		t.Fatalf("expected 1 active character, got %d", len(got))
	}
	if got[0].Name != "  Ana  " {
		t.Fatalf("expected Ana match, got %q", got[0].Name)
	}

	got = detectActiveCharacters(chars, "   ")
	if len(got) != 0 {
		t.Fatalf("expected no matches for empty user message, got %d", len(got))
	}
}

func TestIsNegativeCategory_Coverage(t *testing.T) {
	cases := []struct {
		cat  string
		want bool
	}{
		{"IRA", true},
		{"enojo", true},
		{"alegria", false},
		{"alegría", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isNegativeCategory(tc.cat); got != tc.want {
			t.Fatalf("isNegativeCategory(%q)=%v, want %v", tc.cat, got, tc.want)
		}
	}
}
