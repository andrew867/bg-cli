package publicintel

import (
	"testing"

	"github.com/andrew867/bg-cli/internal/analysis"
)

func TestParseReleasesListAndDiff(t *testing.T) {
	body := []byte(`{
		"status": "ok",
		"data": {
			"releases": [
				{"version": 2, "added": 1, "changed": 0, "collections": {}},
				{"version": 1, "added": 0, "changed": 1, "collections": {"Pets": {"added": 0, "changed": 1}}}
			],
			"total": 2,
			"page": 1,
			"pageSize": 20
		}
	}`)
	a, err := ParseReleasesList(body)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Releases[0].Version.ParseOK || a.Releases[0].Version.Text != "2" {
		t.Fatalf("version: %+v", a.Releases[0].Version)
	}
	if a.Completeness != analysis.Complete {
		t.Fatalf("completeness=%s", a.Completeness)
	}
	bodyB := []byte(`{
		"status": "ok",
		"data": {
			"releases": [
				{"version": 2, "added": 5, "changed": 0, "collections": {}}
			],
			"total": 1,
			"page": 1,
			"pageSize": 20
		}
	}`)
	b, err := ParseReleasesList(bodyB)
	if err != nil {
		t.Fatal(err)
	}
	diff := DiffReleases(a, b)
	if len(diff.OnlyInB) != 0 || len(diff.ChangedVersions) != 1 {
		t.Fatalf("diff=%+v", diff)
	}
}

func TestItemChangesCheckpointAdvance(t *testing.T) {
	page1 := ItemChangesPage{
		Since:         map[string]any{"version": float64(1)},
		LatestVersion: ExactInt{Text: "3", ParseOK: true, Value: int64Ptr(3)},
		Total:         ExactInt{Text: "2", ParseOK: true, Value: int64Ptr(2)},
		Page:          ExactInt{Text: "1", ParseOK: true, Value: int64Ptr(1)},
		PageSize:      ExactInt{Text: "1", ParseOK: true, Value: int64Ptr(1)},
		HasMore:       true,
		Changes:       []ItemChangeRow{{Version: ExactInt{Text: "2", ParseOK: true, Value: int64Ptr(2)}, Collection: "Pets", Change: "added"}},
	}
	page2 := ItemChangesPage{
		LatestVersion: ExactInt{Text: "3", ParseOK: true, Value: int64Ptr(3)},
		Total:         ExactInt{Text: "2", ParseOK: true, Value: int64Ptr(2)},
		Page:          ExactInt{Text: "2", ParseOK: true, Value: int64Ptr(2)},
		PageSize:      ExactInt{Text: "1", ParseOK: true, Value: int64Ptr(1)},
		HasMore:       false,
		Changes:       []ItemChangeRow{{Version: ExactInt{Text: "3", ParseOK: true, Value: int64Ptr(3)}, Collection: "Pets", Change: "changed"}},
	}
	cp := ChangesCheckpoint{SinceVersion: ExactInt{Text: "1", ParseOK: true, Value: int64Ptr(1)}}
	if _, ok := AdvanceChangesCheckpoint(cp, []ItemChangesPage{page1}); ok {
		t.Fatal("should not advance before all pages validated")
	}
	next, ok := AdvanceChangesCheckpoint(cp, []ItemChangesPage{page1, page2})
	if !ok || next.LatestVersion.Text != "3" {
		t.Fatalf("advance failed: %+v ok=%v", next, ok)
	}
}

func TestParseLeaguePlayersSampled(t *testing.T) {
	body := []byte(`{
		"status": "ok",
		"data": {
			"season": "TestSeason",
			"players": [
				{"UserID": 1, "DisplayName": "A", "Points": 10, "League": {"Name": "L", "ID": "x", "Points": 100}}
			]
		}
	}`)
	rep, err := ParseLeaguePlayers(body)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Completeness != analysis.Sampled {
		t.Fatalf("completeness=%s", rep.Completeness)
	}
	if rep.Players[0].UserID.Text != "1" {
		t.Fatal(rep.Players[0])
	}
}

func TestParseClanPlayersPascalCase(t *testing.T) {
	body := []byte(`{
		"status": "ok",
		"data": {
			"players": [{"UserID": 9, "DisplayName": "P", "ActiveBattlePoints": 1, "AllTimeDiamonds": 2, "TotalBattles": 3, "EarnedMedals": 0, "Clan": {"Name": "C", "Icon": "rbxassetid://1", "CountryCode": "US"}}],
			"sampledClans": 25,
			"total": 1
		}
	}`)
	rep, err := ParseClanPlayers(body)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Completeness != analysis.Sampled {
		t.Fatalf("completeness=%s", rep.Completeness)
	}
	if rep.Players[0].Clan.Name != "C" {
		t.Fatal(rep.Players[0])
	}
}

func TestParseLeagueLeaderboardPascalCase(t *testing.T) {
	body := []byte(`{
		"status": "ok",
		"data": {
			"season": "S",
			"leagues": [{"Name": "Top", "NameLower": "top", "ID": "id", "Points": 5, "Members": 4, "MemberCapacity": 4, "ContributorCount": 4, "Level": 1}],
			"total": 1,
			"page": 1,
			"pageSize": 25
		}
	}`)
	rep, err := ParseLeagueLeaderboard(body)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Leagues[0].Name != "Top" || rep.Leagues[0].Points.Text != "5" {
		t.Fatalf("%+v", rep.Leagues[0])
	}
}
