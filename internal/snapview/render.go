package snapview

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/andrew867/bg-cli/internal/clan"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
)

// Bundle is how a ZIP turned out. Nil means a ZIP was not written.
type Bundle struct {
	OK          bool
	Error       string
	Name        string
	Dir         string
	Path        string
	Bytes       int64
	SHA256      string
	Members     int
	MemberNames []string
	Missing     []string
	Complete    bool
}

// View is everything the human summary is allowed to read.
// It does not fetch, and it does not change stored JSON or CSV.
type View struct {
	Summary           clan.Summary
	Manifest          snapshot.Manifest
	Priors            []clan.Prior
	ShowAllCurrencies bool
	Verbose           bool
	Color             bool
	Bundle            *Bundle
	Location          *time.Location
}

// Render writes the default terminal summary. Colour is applied only when
// View.Color is set. The text stays readable without it.
func Render(v View) string {
	var b strings.Builder
	sum := v.Summary
	title := "PS99 snapshot"
	if p := strings.TrimSpace(sum.Prefix); p != "" && !strings.EqualFold(p, "ps99") {
		title = p + " snapshot"
	}
	fmt.Fprintf(&b, "%s  %s\n", title, capturedAt(sum.ExportedAt))
	fmt.Fprintf(&b, "%-14s %s\n", "Profile", textOrNA(sum.Profile))
	fmt.Fprintf(&b, "%-14s %s\n", "User", textOrNA(sum.UserID))
	session := unavailable
	if sum.Windows.Session.Valid {
		session = FormatDuration(sum.Windows.Session.ElapsedSeconds)
	}
	fmt.Fprintf(&b, "%-14s %s\n", "Session", session)
	refresh, reason := NextRefresh(v.Manifest, v.Location)
	fmt.Fprintf(&b, "%-14s %s\n", "Next refresh", refresh)
	if reason != "" {
		fmt.Fprintf(&b, "  %s\n", reason)
	}

	label, win, winOK := displayWindow(sum.Windows)
	b.WriteString("\nAccount\n")
	writeCurrencies(&b, v, label, win, winOK)

	b.WriteString("\nActivity\n")
	writeActivity(&b, v, label, win, winOK)

	b.WriteString("\n")
	writeClan(&b, v, label, win, winOK)

	b.WriteString("\nInventory\n")
	writeInventory(&b, v.Manifest)

	b.WriteString("\nData\n")
	writeSources(&b, v)

	if warnings := collectWarnings(v, label, winOK); len(warnings) > 0 {
		b.WriteString("\nWarnings\n")
		for _, w := range warnings {
			fmt.Fprintf(&b, "  %s\n", oneLine(w))
		}
	}

	b.WriteString("\nFiles\n")
	writeFiles(&b, v)

	if v.Bundle != nil {
		b.WriteString("\n")
		writeBundle(&b, v)
	}
	return b.String()
}

func capturedAt(exported string) string {
	if t, err := time.Parse(time.RFC3339, exported); err == nil {
		return t.Format("2006-01-02 15:04:05")
	}
	if strings.TrimSpace(exported) == "" {
		return unavailable
	}
	return exported
}

// NextRefresh formats nextRefreshEligibleAt from refresh objects already
// stored on the snapshot. It does not invent a time from resetsAt or the clock.
// The reason is empty when a local time is shown.
func NextRefresh(man snapshot.Manifest, loc *time.Location) (shown, reason string) {
	raw, why := nextRefreshRaw(man)
	if raw == "" {
		if why == "" {
			why = "refresh time was not returned"
		}
		return unavailable, why
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return unavailable, "next refresh time could not be read"
	}
	return FormatLocalClock(t, loc), ""
}

func nextRefreshRaw(man snapshot.Manifest) (string, string) {
	var fallback string
	saw := false
	bad := false
	for _, f := range man.Files {
		blob := strings.TrimSpace(string(f.Refresh))
		if blob == "" || blob == "null" {
			continue
		}
		saw = true
		var body struct {
			Next *string `json:"nextRefreshEligibleAt"`
		}
		if err := json.Unmarshal(f.Refresh, &body); err != nil {
			bad = true
			continue
		}
		if body.Next == nil || strings.TrimSpace(*body.Next) == "" {
			continue
		}
		raw := strings.TrimSpace(*body.Next)
		if f.View == "inventory" || f.Kind == "inventory-json" {
			return raw, ""
		}
		if fallback == "" {
			fallback = raw
		}
	}
	if fallback != "" {
		return fallback, ""
	}
	if bad {
		return "", "next refresh time could not be read"
	}
	if !saw {
		return "", "refresh time was not returned"
	}
	return "", "next refresh time was not returned"
}

func textOrNA(s string) string {
	if strings.TrimSpace(s) == "" {
		return unavailable
	}
	return strings.TrimSpace(s)
}

func displayWindow(w clan.Windows) (string, clan.Window, bool) {
	ordered := []struct {
		label string
		win   clan.Window
	}{
		{"previous", w.Previous},
		{"session", w.Session},
		{"1h", w.Hour},
		{"30m", w.Thirty},
		{"5m", w.Five},
	}
	for _, item := range ordered {
		if item.win.Valid {
			return item.label, item.win, true
		}
	}
	return "previous", w.Previous, false
}

func writeCurrencies(b *strings.Builder, v View, label string, win clan.Window, winOK bool) {
	lines := currencyLines(v.Summary, v.Priors, win, winOK, v.ShowAllCurrencies, label)
	if len(lines) == 0 {
		fmt.Fprintf(b, "  %s\n", unavailable)
		return
	}
	for _, line := range lines {
		value := line.amount
		if line.delta != "" {
			value = amountAndDelta(line.amount, line.delta, v.Color, false)
		} else if line.note != "" {
			value = line.amount + "    " + line.note
		}
		row(b, line.name, value)
	}
}

type moneyLine struct {
	name   string
	amount string
	delta  string
	note   string
}

func currencyLines(sum clan.Summary, priors []clan.Prior, win clan.Window, winOK, showAll bool, label string) []moneyLine {
	changed := map[string]clan.CurrencyDelta{}
	if winOK {
		changed = win.ChangedCurrencies
	}
	ids := map[string]struct{}{}
	if showAll || sum.Current.CurrenciesKnown {
		for id := range sum.Current.Currencies {
			if showAll || preferCurrency(id, sum.Current.Currencies[id].DisplayName) || (winOK && changedHas(changed, id)) {
				ids[id] = struct{}{}
			}
		}
	}
	if winOK {
		for id := range changed {
			ids[id] = struct{}{}
		}
	}
	if !sum.Current.CurrenciesKnown && len(ids) == 0 {
		return nil
	}
	names := make([]string, 0, len(ids))
	for id := range ids {
		names = append(names, id)
	}
	sortCurrencyIDs(names)
	var out []moneyLine
	for _, id := range names {
		cur := sum.Current.Currencies[id]
		name := cur.DisplayName
		if strings.TrimSpace(name) == "" {
			name = id
		}
		amount := unavailable
		if cur.Amount != "" {
			amount = FormatCount(cur.Amount)
		}
		line := moneyLine{name: name, amount: amount}
		if winOK {
			if d, ok := changed[id]; ok {
				switch {
				case d.Status == "unavailable" || d.Delta == "":
					note := d.Reason
					if note == "" {
						note = unavailable
					}
					line.note = note
				default:
					line.delta = sinceRates(d.Delta, label, elapsedSeconds(win))
				}
			} else if cur.Amount != "" {
				if base, ok := baselineCurrency(priors, win.BaselineFile, id); ok && clan.SameAmount(base, cur.Amount) {
					line.delta = sinceRates("0", label, elapsedSeconds(win))
				}
			}
		}
		out = append(out, line)
	}
	return out
}

func changedHas(m map[string]clan.CurrencyDelta, id string) bool {
	_, ok := m[id]
	return ok
}

func preferCurrency(id, display string) bool {
	if strings.EqualFold(id, "Diamonds") || strings.EqualFold(display, "Diamonds") {
		return true
	}
	blob := strings.ToLower(id + " " + display)
	for _, word := range []string{"event", "pumpkin", "halloween", "tycoon", "christmas", "easter", "festival"} {
		if strings.Contains(blob, word) {
			return true
		}
	}
	return false
}

func sortCurrencyIDs(ids []string) {
	for i := 1; i < len(ids); i++ {
		j := i
		for j > 0 && currencyLess(ids[j], ids[j-1]) {
			ids[j], ids[j-1] = ids[j-1], ids[j]
			j--
		}
	}
}

func currencyLess(a, b string) bool {
	ar, br := currencyRank(a), currencyRank(b)
	if ar != br {
		return ar < br
	}
	return strings.ToLower(a) < strings.ToLower(b)
}

func currencyRank(id string) int {
	if strings.EqualFold(id, "Diamonds") {
		return 0
	}
	if preferCurrency(id, "") {
		return 1
	}
	return 2
}

func writeActivity(b *strings.Builder, v View, label string, win clan.Window, winOK bool) {
	current := v.Summary.Current.Statistics
	if !v.Summary.Current.StatisticsKnown || len(current) == 0 {
		fmt.Fprintf(b, "  %s\n", unavailable)
		return
	}
	base, baseOK := priorStats(v.Priors, win.BaselineFile)
	rows := []struct {
		label    string
		keys     []string
		duration bool
	}{
		{"Eggs opened", []string{"Eggs Opened"}, false},
		{"Huge pets opened", []string{"Huge Pets Opened"}, false},
		{"Golden pets opened", []string{"Golden Pets Opened"}, false},
		{"Rainbow pets opened", []string{"Rainbow Pets Opened"}, false},
		{"Shiny pets opened", []string{"Shiny Pets Opened"}, false},
		{"Playtime", []string{"Playtime", "Time Played", "Play Time"}, true},
	}
	wrote := false
	var eggsDelta string
	var eggsElapsed int64
	eggsOK := false
	for _, spec := range rows {
		cur, ok := lookupStat(current, spec.keys...)
		if !ok {
			continue
		}
		wrote = true
		amount := FormatCount(cur)
		if spec.duration {
			if n, ok := clan.ParseExactInt(cur); ok && n.IsInt64() && n.Sign() >= 0 {
				s := n.Int64()
				amount = FormatDuration(&s)
			}
		}
		extra := ""
		if spec.label == "Eggs opened" && winOK && win.Eggs != nil {
			if win.Eggs.Status != "unavailable" && win.Eggs.Delta != "" {
				extra = sinceRates(win.Eggs.Delta, label, elapsedSeconds(win))
				eggsDelta = win.Eggs.Delta
				eggsElapsed = elapsedSeconds(win)
				eggsOK = true
			}
		} else if winOK && baseOK {
			if prev, ok := lookupStat(base, spec.keys...); ok {
				if d, ok := clan.Sub(cur, prev); ok {
					if spec.duration {
						if n, ok := clan.ParseExactInt(d); ok && n.IsInt64() {
							s := n.Int64()
							if n.Sign() < 0 {
								s = -s
							}
							sign := "+"
							if n.Sign() < 0 {
								sign = "-"
							}
							if n.Sign() == 0 {
								extra = "0 since " + label
							} else {
								extra = sign + FormatDuration(&s) + " since " + label
							}
						}
					} else if spec.label == "Eggs opened" {
						extra = sinceRates(d, label, elapsedSeconds(win))
					} else {
						extra = FormatSigned(d) + " since " + label
					}
					if spec.label == "Eggs opened" {
						eggsDelta = d
						eggsElapsed = elapsedSeconds(win)
						eggsOK = true
					}
				}
			}
		}
		value := amount
		if extra != "" {
			value = amountAndDelta(amount, extra, v.Color, false)
		}
		row(b, spec.label, value)
	}
	if lookupStatExists(current, "Eggs Opened") {
		rate := unavailable
		if eggsOK {
			rate = FormatRatePerHour(eggsDelta, eggsElapsed) + "    " + FormatRatePerMinute(eggsDelta, eggsElapsed)
		}
		row(b, "Hatch rate", rate)
	}
	if !wrote {
		fmt.Fprintf(b, "  %s\n", unavailable)
	}
}

func lookupStatExists(m map[string]string, key string) bool {
	_, ok := lookupStat(m, key)
	return ok
}

func lookupStat(m map[string]string, keys ...string) (string, bool) {
	for _, want := range keys {
		for k, val := range m {
			if strings.EqualFold(k, want) && strings.TrimSpace(val) != "" {
				return strings.TrimSpace(val), true
			}
		}
	}
	return "", false
}

func priorStats(priors []clan.Prior, file string) (map[string]string, bool) {
	if file == "" {
		return nil, false
	}
	for _, p := range priors {
		if p.File == file && p.StatsKnown {
			return p.Stats, true
		}
	}
	return nil, false
}

func writeClan(b *strings.Builder, v View, label string, win clan.Window, winOK bool) {
	clanNow := v.Summary.Current.Clan
	if strings.TrimSpace(clanNow.Tag) == "" {
		fmt.Fprintf(b, "Clan\n  %s\n", paint(v.Color, "unavailable", ansiYellow))
		if reason := strings.TrimSpace(clanNow.Reason); reason != "" {
			fmt.Fprintf(b, "  %s\n", oneLine(reason))
		}
		return
	}
	head := clanNow.Tag
	if clanNow.Name != "" && !strings.EqualFold(clanNow.Name, clanNow.Tag) {
		head = head + " " + clanNow.Name
	}
	fmt.Fprintf(b, "Clan %s\n", head)
	battle := clanNow.ActiveBattle
	if battle == nil {
		row(b, "Battle", paint(v.Color, unavailable, ansiYellow))
		if reason := strings.TrimSpace(clanNow.Reason); reason != "" {
			fmt.Fprintf(b, "  %s\n", oneLine(reason))
		}
		return
	}
	title := textOrNA(battle.Title)
	if title == unavailable {
		title = textOrNA(battle.ID)
	}
	row(b, "Battle", title)
	placeText := FormatRank(battle.Place)
	if winOK && win.Clan != nil && win.Clan.Status != "unavailable" && win.Clan.Place != nil && win.Clan.Place.Status != "unavailable" && win.Clan.Place.Delta != "" {
		placeText = amountAndDelta(placeText, FormatSigned(win.Clan.Place.Delta)+" since "+label, v.Color, false)
	}
	row(b, "Place", placeText)
	points := unavailable
	if battle.Points != "" {
		points = FormatCount(battle.Points)
	}
	pointsExtra := ""
	you := unavailable
	if battle.UserContribution != "" {
		you = FormatCount(battle.UserContribution)
	}
	youExtra := ""
	if winOK && win.Clan != nil && win.Clan.Status != "unavailable" {
		if win.Clan.Points != nil && win.Clan.Points.Delta != "" && win.Clan.Points.Status != "unavailable" {
			pointsExtra = FormatSigned(win.Clan.Points.Delta) + " since " + label
		}
		if win.Clan.UserContribution != nil && win.Clan.UserContribution.Delta != "" && win.Clan.UserContribution.Status != "unavailable" {
			youExtra = FormatSigned(win.Clan.UserContribution.Delta) + " since " + label
		}
	}
	if pointsExtra != "" {
		row(b, "Clan points", amountAndDelta(points, pointsExtra, v.Color, true))
	} else {
		row(b, "Clan points", points)
	}
	if youExtra != "" {
		row(b, "Your contribution", amountAndDelta(you, youExtra, v.Color, true))
	} else {
		row(b, "Your contribution", you)
	}
	row(b, "Contributor rank", FormatRank(battle.UserContributorRank))
	if winOK && win.Clan != nil && win.Clan.Status == "unavailable" && win.Clan.Reason != "" {
		fmt.Fprintf(b, "  %s\n", oneLine(win.Clan.Reason))
	}
}

func writeInventory(b *strings.Builder, man snapshot.Manifest) {
	inv, _ := man.Summary["inventory"].(map[string]any)
	row(b, "Pet quantity", formatAnyCount(inv["petQuantitySum"]))
	rows := inv["rowCount"]
	if rows == nil {
		rows = inv["petRowCount"]
	}
	row(b, "Inventory rows", formatAnyCount(rows))
}

func formatAnyCount(v any) string {
	switch t := v.(type) {
	case nil:
		return unavailable
	case string:
		return FormatCount(t)
	case int:
		return FormatCount(strconv.Itoa(t))
	case int64:
		return FormatCount(strconv.FormatInt(t, 10))
	case int32:
		return FormatCount(strconv.FormatInt(int64(t), 10))
	case uint64:
		return FormatCount(strconv.FormatUint(t, 10))
	case float64:
		// A JSON object decoded into map[string]any may already be a float.
		// Only print it when it is an exact small integer. Otherwise say n/a
		// instead of inventing a rounded count.
		if math.IsNaN(t) || math.IsInf(t, 0) || t < 0 || t > 1<<53 || t != math.Trunc(t) {
			return unavailable
		}
		return FormatCount(strconv.FormatInt(int64(t), 10))
	default:
		return FormatCount(fmt.Sprint(t))
	}
}

func writeSources(b *strings.Builder, v View) {
	type spec struct {
		label string
		kind  string
		view  string
	}
	wanted := []spec{
		{"Inventory", "inventory-json", "inventory"},
		{"Profile", "profile-json", "profile"},
		{"Extended profile", "extended-profile-json", "extendedProfile"},
		{"Clan", "clan-json", ""},
		{"Clan battle", "clan-battle-json", ""},
	}
	seenKind := map[string]bool{}
	seenView := map[string]bool{}
	for _, item := range wanted {
		seenKind[item.kind] = true
		if item.view != "" {
			seenView[item.view] = true
		}
		writeSource(b, v, item.label, item.kind, item.view)
	}
	for _, f := range v.Manifest.Files {
		if f.Kind == "" || seenKind[f.Kind] || f.View == "" || seenView[f.View] {
			continue
		}
		seenKind[f.Kind] = true
		seenView[f.View] = true
		writeSource(b, v, f.View, f.Kind, f.View)
	}
}

func writeSource(b *strings.Builder, v View, label, kind, view string) {
	var matches []snapshot.FileRecord
	for _, f := range v.Manifest.Files {
		if view != "" && f.View == view {
			matches = append(matches, f)
			continue
		}
		if f.Kind == kind {
			matches = append(matches, f)
		}
	}
	status, detail := sourceStatus(v, kind, matches)
	row(b, label, paint(v.Color, status, statusColor(status)))
	if detail != "" && status != "OK" && status != "cached" {
		fmt.Fprintf(b, "    %s\n", oneLine(detail))
	}
}

func sourceStatus(v View, kind string, matches []snapshot.FileRecord) (string, string) {
	if len(matches) == 0 {
		switch kind {
		case "clan-json":
			reason := strings.TrimSpace(v.Summary.Current.Clan.Reason)
			if reason == "" {
				reason = "clan was not captured"
			}
			return "unavailable", reason
		case "clan-battle-json":
			if v.Summary.Current.Clan.ActiveBattle != nil {
				return "unavailable", "clan battle was not captured"
			}
			if strings.TrimSpace(v.Summary.Current.Clan.Tag) == "" {
				reason := strings.TrimSpace(v.Summary.Current.Clan.Reason)
				if reason == "" {
					reason = "clan tag is absent"
				}
				return "unavailable", reason
			}
			reason := strings.TrimSpace(v.Summary.Current.Clan.Reason)
			if reason == "" {
				reason = "no battle is active"
			}
			return "unavailable", reason
		default:
			return "skipped", "not requested"
		}
	}
	var failed *snapshot.FileRecord
	cached := false
	quota := false
	for i := range matches {
		f := &matches[i]
		if !f.OK {
			if failed == nil {
				failed = f
			}
			continue
		}
		if f.FromLocalCache != nil && *f.FromLocalCache {
			cached = true
		}
		if f.QuotaExhausted != nil && *f.QuotaExhausted {
			quota = true
		}
		switch f.ContentStatus {
		case snapshot.ContentRateLimited:
			return "failed", "rate limited"
		case snapshot.ContentScopeDenied:
			return "failed", "not authorized"
		case snapshot.ContentRequestFailed, snapshot.ContentInvalid:
			if failed == nil {
				failed = f
			}
		case snapshot.ContentUpstreamNull, snapshot.ContentOmitted:
			return "unavailable", "upstream sent no document"
		}
	}
	if failed != nil {
		msg := failed.Error
		if msg == "" {
			msg = "request failed"
		}
		return "failed", msg
	}
	if quota {
		return "quota exhausted", "refresh quota was exhausted"
	}
	if cached {
		return "cached", ""
	}
	return "OK", ""
}

func statusColor(status string) string {
	switch status {
	case "OK", "cached":
		return ansiGreen
	case "unavailable":
		return ansiYellow
	case "failed", "quota exhausted":
		return ansiRed
	default:
		return ""
	}
}

func collectWarnings(v View, used string, usedOK bool) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = oneLine(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, w := range v.Summary.Warnings {
		add(w)
	}
	if !v.Summary.Windows.Previous.Valid && used != "previous" {
		add(windowWarning("Previous", v.Summary.Windows.Previous))
	}
	if !v.Summary.Windows.Session.Valid {
		add(windowWarning("Session", v.Summary.Windows.Session))
	}
	if cw := v.Summary.Windows.Session.Clan; v.Summary.Windows.Session.Valid && cw != nil && cw.Status == "unavailable" {
		if cw.BaselineBattleID == "" || strings.Contains(cw.Reason, "baseline has no") || strings.Contains(cw.Reason, "clan tag is absent") {
			add("Session clan delta unavailable because the session baseline predates clan metadata.")
		} else if cw.Reason != "" {
			add("Session clan delta unavailable: " + cw.Reason)
		}
	}
	if !usedOK {
		add(windowWarning("Previous", v.Summary.Windows.Previous))
	}
	if v.Summary.Current.StatisticsKnown && lookupStatExists(v.Summary.Current.Statistics, "Eggs Opened") && usedOK {
		if _, ok := priorStats(v.Priors, displayBaseline(v)); !ok {
			add("Hatch rate unavailable because the baseline has no Eggs Opened statistic.")
		}
	}
	if v.Bundle != nil && !v.Bundle.OK {
		msg := v.Bundle.Error
		if msg == "" {
			msg = "ZIP failed"
		}
		add("ZIP failed: " + msg)
	}
	if v.Bundle != nil && v.Bundle.OK && !v.Bundle.Complete {
		add("ZIP is incomplete; a listed snapshot file was missing.")
	}
	return out
}

func displayBaseline(v View) string {
	_, win, ok := displayWindow(v.Summary.Windows)
	if !ok {
		return ""
	}
	return win.BaselineFile
}

func windowWarning(label string, w clan.Window) string {
	if w.Valid {
		return ""
	}
	reason := w.Reason
	if reason == "" {
		reason = "unavailable"
	}
	return label + " window unavailable: " + reason
}

func writeFiles(b *strings.Builder, v View) {
	if v.Verbose && strings.TrimSpace(v.Manifest.OutputDir) != "" {
		fmt.Fprintf(b, "  %s\n", v.Manifest.OutputDir)
	}
	for _, f := range v.Manifest.Files {
		name := f.Name
		if name == "" {
			name = f.RelPath
		}
		if name == "" {
			continue
		}
		if v.Verbose && v.Manifest.OutputDir != "" {
			fmt.Fprintf(b, "  %s\n", strings.TrimRight(v.Manifest.OutputDir, `/\`)+"/"+name)
		} else {
			fmt.Fprintf(b, "  %s\n", name)
		}
	}
	if v.Manifest.ManifestFile != "" {
		if v.Verbose && v.Manifest.OutputDir != "" {
			fmt.Fprintf(b, "  %s\n", strings.TrimRight(v.Manifest.OutputDir, `/\`)+"/"+v.Manifest.ManifestFile)
		} else {
			fmt.Fprintf(b, "  %s\n", v.Manifest.ManifestFile)
		}
	}
	if v.Bundle != nil && v.Bundle.OK && v.Bundle.Name != "" {
		if v.Verbose && v.Bundle.Path != "" {
			fmt.Fprintf(b, "  %s\n", v.Bundle.Path)
		} else {
			fmt.Fprintf(b, "  %s\n", v.Bundle.Name)
		}
	}
}

func writeBundle(b *strings.Builder, v View) {
	bun := v.Bundle
	fmt.Fprintf(b, "Bundle\n")
	if !bun.OK {
		row(b, "ZIP", paint(v.Color, "failed", ansiRed))
		if bun.Error != "" {
			fmt.Fprintf(b, "  %s\n", oneLine(bun.Error))
		}
		return
	}
	dir := bun.Dir
	if !v.Verbose {
		dir = baseName(dir)
	}
	name := bun.Name
	if v.Verbose && bun.Path != "" {
		name = bun.Path
	}
	row(b, "Files", strconv.Itoa(bun.Members))
	if dir != "" {
		row(b, "Directory", dir)
	}
	if name != "" {
		row(b, "Name", name)
	}
	row(b, "Size", formatBytes(bun.Bytes))
	if bun.SHA256 != "" {
		row(b, "sha256", shortHash(bun.SHA256))
	}
	if !bun.Complete {
		fmt.Fprintf(b, "  %s\n", paint(v.Color, "incomplete", ansiYellow))
		for _, miss := range bun.Missing {
			fmt.Fprintf(b, "  missing %s\n", miss)
		}
	}
}

func baseName(path string) string {
	path = strings.TrimRight(path, `/\`)
	if path == "" {
		return ""
	}
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

func formatBytes(n int64) string {
	if n < 0 {
		return unavailable
	}
	if n < 10000 {
		return withCommas(itoa(n)) + " bytes"
	}
	const unit = 1000
	div := int64(1)
	suf := "bytes"
	for _, step := range []struct {
		lim int64
		suf string
	}{
		{unit, "KB"},
		{unit * unit, "MB"},
		{unit * unit * unit, "GB"},
	} {
		if n >= step.lim/10*10 && n >= 10000 {
			div = step.lim
			suf = step.suf
		}
	}
	if suf == "bytes" {
		return withCommas(itoa(n)) + " bytes"
	}
	whole := n / div
	frac := (n % div) * 10 / div
	return itoa(whole) + "." + itoa(frac) + " " + suf
}

func shortHash(sum string) string {
	if len(sum) <= 12 {
		return sum
	}
	return sum[:12] + "…"
}

func elapsedSeconds(win clan.Window) int64 {
	if win.ElapsedSeconds == nil || *win.ElapsedSeconds <= 0 {
		return 0
	}
	return *win.ElapsedSeconds
}

func sinceRates(delta, label string, elapsed int64) string {
	change := FormatSigned(delta)
	if change == unavailable {
		return unavailable
	}
	hour := unavailable
	minute := unavailable
	if elapsed > 0 {
		hour = FormatRatePerHour(delta, elapsed)
		minute = FormatRatePerMinute(delta, elapsed)
	}
	return change + " since " + label + "    " + hour + "    " + minute
}

func baselineCurrency(priors []clan.Prior, file, id string) (string, bool) {
	if file == "" {
		return "", false
	}
	for _, p := range priors {
		if p.File != file || !p.CurrenciesKnown {
			continue
		}
		v, ok := p.Currencies[id]
		if !ok || strings.TrimSpace(v) == "" {
			return "", false
		}
		return v, true
	}
	return "", false
}

func amountAndDelta(amount, delta string, color, badNegative bool) string {
	if delta == "" {
		return amount
	}
	code := ""
	if color {
		if strings.HasPrefix(delta, "+") {
			code = ansiGreen
		} else if badNegative && strings.HasPrefix(delta, "-") {
			code = ansiRed
		}
	}
	if code == "" {
		return amount + "    " + delta
	}
	return amount + "    " + code + delta + ansiReset
}

func row(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "  %-22s %s\n", label, value)
}

func paint(on bool, text, code string) string {
	if !on || code == "" || text == "" {
		return text
	}
	return code + text + ansiReset
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 180 {
		s = s[:177] + "..."
	}
	return s
}
