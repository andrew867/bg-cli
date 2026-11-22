package snapshot

import "testing"

func TestProfileFileName(t *testing.T) {
	if got := ProfileFileName("2026-11-22-120000", "", "", "ps99", "inv.json"); got != "2026-11-22-120000-ps99-inv.json" {
		t.Fatalf("single %s", got)
	}
	if got := ProfileFileName("2026-11-22-120000", "0001", "", "ps99", "manifest.json"); got != "2026-11-22-120000-0001-ps99-manifest.json" {
		t.Fatalf("collision %s", got)
	}
	if got := ProfileFileName("2026-11-22-120000", "", "alt1", "ps99", "inv.json"); got != "2026-11-22-120000-alt1-ps99-inv.json" {
		t.Fatalf("profile %s", got)
	}
	if got := ProfileFileName("2026-11-22-120000", "0001", "main", "ps99", "summary.json"); got != "2026-11-22-120000-0001-main-ps99-summary.json" {
		t.Fatalf("profile collision %s", got)
	}
}

func TestManifestNameFollowsSummaryStem(t *testing.T) {
	got := manifestNameFromSummary("2026-11-22-120000-alt1-ps99-summary.json", "2026-11-22-120000", "", "ps99")
	if got != "2026-11-22-120000-alt1-ps99-manifest.json" {
		t.Fatalf("multi %s", got)
	}
	got = manifestNameFromSummary("2026-11-22-120000-ps99-summary.json", "2026-11-22-120000", "", "ps99")
	if got != "2026-11-22-120000-ps99-manifest.json" {
		t.Fatalf("single %s", got)
	}
}
