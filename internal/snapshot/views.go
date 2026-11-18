package snapshot

import "strings"

// AccountViewIDs are the seven documented authenticated account views.
var AccountViewIDs = []string{
	"inventory", "profile", "extendedProfile", "itemIndex", "trades", "booth", "mail",
}

// DefaultSnapshotViews are written by account snapshot without --all-views/--include.
var DefaultSnapshotViews = []string{"inventory", "profile", "extendedProfile"}

// SensitiveViews require explicit --all-views or --include (privacy).
var SensitiveViews = map[string]bool{
	"itemIndex": true, // collection history; opt-in with expanded snapshots
	"trades":    true,
	"booth":     true,
	"mail":      true,
}

// RequestStatus distinguishes why a view file is present or absent.
type RequestStatus string

const (
	RequestRequested RequestStatus = "requested"
	RequestOmitted   RequestStatus = "not_requested"
)

// ContentStatus classifies payload outcome for a requested view.
type ContentStatus string

const (
	ContentOK            ContentStatus = "ok"
	ContentValidEmpty    ContentStatus = "valid_empty"
	ContentUpstreamNull  ContentStatus = "upstream_null"
	ContentScopeDenied   ContentStatus = "scope_denied"
	ContentRateLimited   ContentStatus = "rate_limited"
	ContentRequestFailed ContentStatus = "request_failed"
	ContentInvalid       ContentStatus = "invalid_content"
	ContentOmitted       ContentStatus = "omitted"
)

// ParseIncludeViews parses a comma-separated include list. Empty → default three views.
// "all" → all seven views.
func ParseIncludeViews(include string, allViews bool) ([]string, error) {
	if allViews {
		out := make([]string, len(AccountViewIDs))
		copy(out, AccountViewIDs)
		return out, nil
	}
	include = strings.TrimSpace(include)
	if include == "" {
		out := make([]string, len(DefaultSnapshotViews))
		copy(out, DefaultSnapshotViews)
		return out, nil
	}
	if strings.EqualFold(include, "all") {
		out := make([]string, len(AccountViewIDs))
		copy(out, AccountViewIDs)
		return out, nil
	}
	parts := strings.Split(include, ",")
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		ok := false
		for _, id := range AccountViewIDs {
			if strings.EqualFold(p, id) {
				p = id
				ok = true
				break
			}
		}
		if !ok {
			return nil, ErrUnknownView(p)
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, ErrUnknownView("(empty)")
	}
	return out, nil
}

// ErrUnknownView is returned for invalid --include tokens.
type ErrUnknownView string

func (e ErrUnknownView) Error() string { return "unknown account view: " + string(e) }

// KindForView maps a view id to the on-disk FileRecord.Kind used historically.
func KindForView(view string) string {
	switch view {
	case "inventory":
		return "inventory-json"
	case "profile":
		return "profile-json"
	case "extendedProfile":
		return "extended-profile-json"
	case "itemIndex":
		return "item-index-json"
	case "trades":
		return "trades-json"
	case "booth":
		return "booth-json"
	case "mail":
		return "mail-json"
	default:
		return view + "-json"
	}
}

// SuffixForView is the filename suffix after prefix.
func SuffixForView(view string) string {
	switch view {
	case "inventory":
		return "inv.json"
	case "profile":
		return "prof.json"
	case "extendedProfile":
		return "extprof.json"
	case "itemIndex":
		return "itemindex.json"
	case "trades":
		return "trades.json"
	case "booth":
		return "booth.json"
	case "mail":
		return "mail.json"
	default:
		return view + ".json"
	}
}
