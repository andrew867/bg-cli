package accountanalytics

import (
	"encoding/json"
	"testing"
)

func TestProgressionAchievementStages(t *testing.T) {
	from := json.RawMessage(`{
		"Rank": 10,
		"Achievements": {
			"EggHunter": {"stage": 2},
			"ZoneRush": {"stage": 5}
		},
		"Mastery": {"Wood": 100}
	}`)
	to := json.RawMessage(`{
		"Rank": 12,
		"Achievements": {
			"EggHunter": {"stage": 3},
			"ZoneRush": {"stage": 4}
		},
		"Mastery": {"Wood": 150, "Rock": 20}
	}`)
	diff, err := DiffProgression(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Scalars) != 1 || diff.Scalars[0].Field != "Rank" || diff.Scalars[0].Delta != "2" {
		t.Fatalf("scalars: %+v", diff.Scalars)
	}
	var advanced, regressed bool
	for _, a := range diff.Achievements {
		switch a.ChangeType {
		case "advanced":
			advanced = true
		case "regressed":
			regressed = true
		}
	}
	if !advanced || !regressed {
		t.Fatalf("achievements: %+v", diff.Achievements)
	}
	if len(diff.MapChanges) < 2 {
		t.Fatalf("map changes: %+v", diff.MapChanges)
	}
}
