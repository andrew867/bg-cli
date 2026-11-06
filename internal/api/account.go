package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Scope constants for account views.
const (
	ScopeInventory       = "player-data:pet-simulator-99:inventory:read"
	ScopeProfile         = "player-data:pet-simulator-99:profile:read"
	ScopeExtendedProfile = "player-data:pet-simulator-99:extendedProfile:read"
	ScopeItemIndex       = "player-data:pet-simulator-99:itemIndex:read"
	ScopeTrades          = "player-data:pet-simulator-99:trades:read"
	ScopeBooth           = "player-data:pet-simulator-99:booth:read"
	ScopeMail            = "player-data:pet-simulator-99:mail:read"
)

// PathForView maps a command view to its allowlisted path and required scope.
func PathForView(view string) (path, scope string, ok bool) {
	switch view {
	case "inventory":
		return "/v1/account/inventory", ScopeInventory, true
	case "profile":
		return "/v1/account/profile", ScopeProfile, true
	case "extendedProfile":
		return "/v1/account/extendedProfile", ScopeExtendedProfile, true
	case "itemIndex":
		return "/v1/account/itemIndex", ScopeItemIndex, true
	case "trades":
		return "/v1/account/trades", ScopeTrades, true
	case "booth":
		return "/v1/account/booth", ScopeBooth, true
	case "mail":
		return "/v1/account/mail", ScopeMail, true
	default:
		return "", "", false
	}
}

// AccountGetOptions controls an account GET.
type AccountGetOptions struct {
	Refresh bool
}

func (c *Client) getAccountView(ctx context.Context, path, token string, opt AccountGetOptions, limit int64) (Result, error) {
	q := url.Values{}
	if opt.Refresh {
		q.Set("refresh", "true")
	}
	return c.GetAccount(ctx, path, q, token, limit)
}

// GetInventory fetches /v1/account/inventory.
func (c *Client) GetInventory(ctx context.Context, token string, opt AccountGetOptions) (Result, error) {
	return c.getAccountView(ctx, "/v1/account/inventory", token, opt, BodyLimitInventory)
}

// GetProfile fetches /v1/account/profile.
func (c *Client) GetProfile(ctx context.Context, token string, opt AccountGetOptions) (Result, error) {
	return c.getAccountView(ctx, "/v1/account/profile", token, opt, BodyLimitDefault)
}

// GetAccountView fetches an allowlisted account path.
func (c *Client) GetAccountView(ctx context.Context, path, token string, opt AccountGetOptions) (Result, error) {
	if !IsAccountPath(path) {
		return Result{}, fmt.Errorf("path %q is not an allowlisted account route", path)
	}
	return c.getAccountView(ctx, path, token, opt, BodyLimitDefault)
}

// InventoryData is a partial decode for filtering and snapshot fields.
type InventoryData struct {
	Items     []json.RawMessage `json:"items"`
	Equipped  json.RawMessage   `json:"equipped"`
	FetchedAt *time.Time        `json:"fetchedAt"`
	Cached    *bool             `json:"cached"`
}

type inventoryItemMeta struct {
	Class string          `json:"class"`
	ID    string          `json:"id"`
	Count json.RawMessage `json:"count"`
}

func countAsFloat(raw json.RawMessage) float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	return 0
}

// FilterInventory applies exact case-insensitive class/id filters.
// quantitySum is distinct from rowCount and may exceed int64 for game-native stacks.
func FilterInventory(data json.RawMessage, class, id string, equippedOnly bool) (filtered json.RawMessage, rowCount int, quantitySum float64, err error) {
	var inv InventoryData
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, 0, 0, err
	}
	out := make([]json.RawMessage, 0, len(inv.Items))
	for _, raw := range inv.Items {
		var meta inventoryItemMeta
		_ = json.Unmarshal(raw, &meta)
		if class != "" && !strings.EqualFold(meta.Class, class) {
			continue
		}
		if id != "" && !strings.EqualFold(meta.ID, id) {
			continue
		}
		out = append(out, raw)
		quantitySum += countAsFloat(meta.Count)
	}
	if equippedOnly {
		eq := string(inv.Equipped)
		kept := out[:0]
		for _, raw := range out {
			var meta inventoryItemMeta
			_ = json.Unmarshal(raw, &meta)
			if meta.ID != "" && strings.Contains(strings.ToLower(eq), `"`+strings.ToLower(meta.ID)+`"`) {
				kept = append(kept, raw)
			}
		}
		out = kept
		quantitySum = 0
		for _, raw := range out {
			var meta inventoryItemMeta
			_ = json.Unmarshal(raw, &meta)
			quantitySum += countAsFloat(meta.Count)
		}
	}
	inv.Items = out
	b, err := json.Marshal(inv)
	if err != nil {
		return nil, 0, 0, err
	}
	return b, len(out), quantitySum, nil
}

// SnapshotFromInventory returns fetchedAt/cached without inventing values.
func SnapshotFromInventory(data json.RawMessage) (snapshotAt *time.Time, cached *bool) {
	var inv InventoryData
	if json.Unmarshal(data, &inv) != nil {
		return nil, nil
	}
	return inv.FetchedAt, inv.Cached
}

// CurrencyExtraction is the account currencies view.
type CurrencyExtraction struct {
	CurrencyFieldPresent bool                       `json:"currencyFieldPresent"`
	Currencies           map[string]json.RawMessage `json:"currencies"`
	EntryCount           int                        `json:"entryCount"`
}

// ExtractCurrencies pulls data.Currency exactly; missing field is not zero.
func ExtractCurrencies(profileData json.RawMessage) (CurrencyExtraction, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(profileData, &top); err != nil {
		return CurrencyExtraction{}, err
	}
	raw, ok := top["Currency"]
	if !ok || string(raw) == "null" {
		return CurrencyExtraction{
			CurrencyFieldPresent: false,
			Currencies:           map[string]json.RawMessage{},
			EntryCount:           0,
		}, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return CurrencyExtraction{}, err
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	return CurrencyExtraction{
		CurrencyFieldPresent: true,
		Currencies:           m,
		EntryCount:           len(m),
	}, nil
}

// FilterCurrenciesByID keeps entries whose map key or inner id matches (case-insensitive).
func FilterCurrenciesByID(ext CurrencyExtraction, id string) CurrencyExtraction {
	if id == "" || !ext.CurrencyFieldPresent {
		return ext
	}
	out := map[string]json.RawMessage{}
	for k, v := range ext.Currencies {
		var inner struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(v, &inner)
		if strings.EqualFold(k, id) || strings.EqualFold(inner.ID, id) {
			out[k] = v
		}
	}
	ext.Currencies = out
	ext.EntryCount = len(out)
	return ext
}
