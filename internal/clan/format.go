package clan

import (
	"fmt"
	"math/big"
	"strings"
)

// FormatConsole renders the short stderr summary. Unchanged currencies are omitted.
// JSON remains the authoritative record.
func FormatConsole(sum Summary) string {
	var b strings.Builder
	clan := sum.Current.Clan
	battle := clan.ActiveBattle
	if battle != nil && battle.ID != "" {
		fmt.Fprintf(&b, "Active battle: %s\n", battle.ID)
		if battle.Title != "" {
			fmt.Fprintf(&b, "Title: %s\n", battle.Title)
		}
	} else {
		reason := clan.Reason
		if reason == "" {
			reason = "no battle is active"
		}
		fmt.Fprintf(&b, "Active battle: unresolved (%s)\n", reason)
	}
	tag := clan.Tag
	if tag == "" {
		tag = "unresolved"
	}
	name := clan.Name
	if name != "" && name != tag {
		tag = tag + " " + name
	}
	place := "unresolved"
	points := "unresolved"
	you := "unresolved"
	rank := "unresolved"
	if battle != nil {
		if battle.Place != nil {
			place = fmt.Sprintf("#%d", *battle.Place)
		}
		if battle.Points != "" {
			points = formatCount(battle.Points)
		}
		if battle.UserContribution != "" {
			you = formatCount(battle.UserContribution)
		}
		if battle.UserContributorRank != nil {
			rank = fmt.Sprintf("#%d", *battle.UserContributorRank)
		}
	}
	fmt.Fprintf(&b, "Clan: %s  Place %s  Points %s\n", tag, place, points)
	fmt.Fprintf(&b, "You: %s points  Clan contributor %s\n", you, rank)
	if clan.MemberCount != nil {
		fmt.Fprintf(&b, "Members: %d\n", *clan.MemberCount)
	}

	pref, prefName := preferredWindow(sum.Windows)
	b.WriteString("\nChanged currencies:\n")
	if pref == nil || !pref.Valid || (pref.CurrenciesStatus == "unavailable" && len(pref.ChangedCurrencies) == 0) {
		if pref != nil && pref.Valid && pref.CurrenciesReason != "" {
			fmt.Fprintf(&b, "(%s)\n", pref.CurrenciesReason)
		} else {
			b.WriteString("(none)\n")
		}
	} else if len(pref.ChangedCurrencies) == 0 {
		b.WriteString("(none)\n")
	} else {
		elapsed := int64(0)
		if pref.ElapsedSeconds != nil {
			elapsed = *pref.ElapsedSeconds
		}
		ids := sortedKeys(pref.ChangedCurrencies)
		for _, id := range ids {
			row := pref.ChangedCurrencies[id]
			if row.Status == "unavailable" {
				fmt.Fprintf(&b, "%s  %s\n", id, row.Reason)
				continue
			}
			fmt.Fprintf(&b, "%s  %s  %s / %s  %s/h net est.\n",
				id, formatCount(row.Current), formatSigned(row.Delta), formatElapsed(elapsed), formatRate(row.NetPerHour, row.NetPerHourApprox))
		}
		_ = prefName
	}
	b.WriteString("\n")
	writeWindowLine(&b, "5m", sum.Windows.Five)
	writeWindowLine(&b, "30m", sum.Windows.Thirty)
	writeWindowLine(&b, "1h", sum.Windows.Hour)
	writeWindowLine(&b, "Session", sum.Windows.Session)
	writeWindowLine(&b, "Previous", sum.Windows.Previous)
	return b.String()
}

func preferredWindow(w Windows) (*Window, string) {
	ordered := []struct {
		name string
		w    Window
	}{
		{"5m", w.Five},
		{"30m", w.Thirty},
		{"1h", w.Hour},
		{"session", w.Session},
		{"previous", w.Previous},
	}
	for i := range ordered {
		if ordered[i].w.Valid {
			return &ordered[i].w, ordered[i].name
		}
	}
	return nil, ""
}

func writeWindowLine(b *strings.Builder, label string, w Window) {
	if !w.Valid {
		reason := w.Reason
		if reason == "" {
			reason = "unavailable"
		}
		fmt.Fprintf(b, "%s: unavailable (%s)\n", label, reason)
		return
	}
	elapsed := int64(0)
	if w.ElapsedSeconds != nil {
		elapsed = *w.ElapsedSeconds
	}
	clan := "clan points unavailable"
	if w.Clan != nil && w.Clan.Status == "" && w.Clan.Points != nil && w.Clan.Points.Delta != "" {
		clan = fmt.Sprintf("clan %s / %s  %s/h net", formatSigned(w.Clan.Points.Delta), formatElapsed(elapsed), formatRate(w.Clan.Points.NetPerHour, w.Clan.Points.NetPerHourApprox))
	} else if w.Clan != nil && w.Clan.Reason != "" {
		clan = w.Clan.Reason
	}
	fmt.Fprintf(b, "%s: %s, %d currencies changed\n", label, clan, len(w.ChangedCurrencies))
}

func sortedKeys(m map[string]CurrencyDelta) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		j := i
		for j > 0 && keys[j] < keys[j-1] {
			keys[j], keys[j-1] = keys[j-1], keys[j]
			j--
		}
	}
	return keys
}

func formatElapsed(seconds int64) string {
	if seconds < 0 {
		seconds = -seconds
	}
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%dh%dm", seconds/3600, (seconds%3600)/60)
}

func formatRate(exact, approx string) string {
	if approx != "" && approx != exact {
		if strings.HasPrefix(exact, "-") || strings.HasPrefix(approx, "-") {
			return approx + " (" + exact + ")"
		}
		return "+" + approx + " (" + exact + ")"
	}
	return formatSigned(exact)
}

func formatSigned(s string) string {
	if s == "" {
		return "unresolved"
	}
	if strings.Contains(s, "/") {
		if strings.HasPrefix(s, "-") {
			return s
		}
		return "+" + s
	}
	n, ok := ParseExactInt(s)
	if !ok {
		return s
	}
	sign := "+"
	if n.Sign() < 0 {
		sign = "-"
		n = new(big.Int).Abs(n)
	} else if n.Sign() == 0 {
		sign = ""
	}
	return sign + formatCount(n.String())
}

func formatCount(s string) string {
	n, ok := ParseExactInt(s)
	if !ok {
		return s
	}
	neg := n.Sign() < 0
	if neg {
		n = new(big.Int).Abs(n)
	}
	digits := n.String()
	var text string
	if len(digits) > 12 {
		text = formatScaled(digits)
	} else {
		text = withCommas(digits)
	}
	if neg {
		return "-" + text
	}
	return text
}

func withCommas(digits string) string {
	n := len(digits)
	if n <= 3 {
		return digits
	}
	lead := n % 3
	if lead == 0 {
		lead = 3
	}
	var b strings.Builder
	b.WriteString(digits[:lead])
	for i := lead; i < n; i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func formatScaled(digits string) string {
	// One decimal of the largest 1000^k group, display only.
	groups := []struct {
		pow int
		suf string
	}{
		{15, "Q"},
		{12, "T"},
		{9, "B"},
		{6, "M"},
		{3, "K"},
	}
	for _, g := range groups {
		if len(digits) > g.pow {
			whole := digits[:len(digits)-g.pow]
			frac := digits[len(digits)-g.pow:]
			if len(whole) > 6 {
				return withCommas(whole) + g.suf
			}
			tenth := "0"
			if len(frac) > 0 {
				tenth = frac[:1]
			}
			roundUp := len(frac) > 1 && frac[1] >= '5'
			if roundUp {
				whole, tenth = roundOneDecimal(whole, tenth)
			}
			return whole + "." + tenth + g.suf
		}
	}
	return withCommas(digits)
}

func roundOneDecimal(whole, tenth string) (string, string) {
	if tenth != "9" {
		return whole, string(tenth[0] + 1)
	}
	n, ok := new(big.Int).SetString(whole, 10)
	if !ok {
		return whole, "9"
	}
	n.Add(n, big.NewInt(1))
	return n.String(), "0"
}
