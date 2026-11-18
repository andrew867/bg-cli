package clan

import (
	"encoding/json"
	"strings"

	"github.com/andrew867/bg-cli/internal/api"
)

// tagKeys are top-level profile fields that may hold the current clan tag.
// Nested pet names are not searched.
var tagKeys = []string{
	"Clan", "ClanTag", "ClanName", "ClanID", "ClanId",
	"Guild", "GuildTag", "GuildName", "GuildID", "GuildId",
}

// ResolveClanTag chooses the clan tag for a public clan read.
// An explicit flag wins, then a tag discovered on the account documents,
// then the stored profile ps99ClanTag. There is no built-in tag.
func ResolveClanTag(flag, discovered, configured string) (tag, source string, ok bool) {
	if s := strings.TrimSpace(flag); s != "" {
		return s, "flag", true
	}
	if s := strings.TrimSpace(discovered); s != "" {
		return s, "profile", true
	}
	if s := strings.TrimSpace(configured); s != "" {
		return s, "config", true
	}
	return "", "", false
}

// ExtractTag returns the first clan tag on the profile object, then the
// extended-profile object. The value must be a single safe path segment.
func ExtractTag(profile, extended json.RawMessage) (tag, field string, ok bool) {
	if tag, field, ok = tagFromObject(profile); ok {
		return tag, field, true
	}
	return tagFromObject(extended)
}

func tagFromObject(raw json.RawMessage) (string, string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", "", false
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return "", "", false
	}
	for _, key := range tagKeys {
		rawField, exists := top[key]
		if !exists || string(rawField) == "null" {
			continue
		}
		if tag, ok := tagValue(rawField); ok {
			if api.ValidateClanTag(tag) == nil {
				return tag, key, true
			}
		}
	}
	return "", "", false
}

func tagValue(raw json.RawMessage) (string, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return "", false
	}
	if s[0] == '"' {
		var str string
		if json.Unmarshal(raw, &str) != nil {
			return "", false
		}
		str = strings.TrimSpace(str)
		return str, str != ""
	}
	if s[0] == '{' {
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			return "", false
		}
		for _, key := range []string{"Name", "Tag", "ID", "Id", "name", "tag"} {
			if str, ok := jsonString(obj[key]); ok && strings.TrimSpace(str) != "" {
				return strings.TrimSpace(str), true
			}
		}
		return "", false
	}
	if n, ok := ParseExactInt(s); ok {
		return n.String(), true
	}
	return "", false
}
