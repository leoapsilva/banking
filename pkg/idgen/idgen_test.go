package idgen

import "testing"

func TestExternalReferenceID_Length(t *testing.T) {
	id := ExternalReferenceID("sub-123", "2026-06")
	if len(id) != 10 {
		t.Fatalf("expected length 10, got %d (%q)", len(id), id)
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z')) {
			t.Fatalf("id contains invalid character %q in %q", c, id)
		}
	}
}

func TestExternalReferenceID_Deterministic(t *testing.T) {
	a := ExternalReferenceID("sub-123", "2026-06")
	b := ExternalReferenceID("sub-123", "2026-06")
	if a != b {
		t.Fatalf("expected deterministic output, got %q and %q", a, b)
	}
}

func TestExternalReferenceID_DifferentInputsDiffer(t *testing.T) {
	a := ExternalReferenceID("sub-123", "2026-06")
	b := ExternalReferenceID("sub-123", "2026-07")
	if a == b {
		t.Fatalf("expected different cycles to produce different ids, both were %q", a)
	}
}
