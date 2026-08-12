package awsinventory

import (
	"strings"
)

func filterRegions(all, allow, deny []string) []string {
	set := map[string]struct{}{}
	for _, r := range all {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		set[r] = struct{}{}
	}
	if len(allow) > 0 {
		allowed := map[string]struct{}{}
		for _, r := range allow {
			r = strings.TrimSpace(r)
			if r != "" {
				allowed[r] = struct{}{}
			}
		}
		for r := range set {
			if _, ok := allowed[r]; !ok {
				delete(set, r)
			}
		}
	}
	for _, r := range deny {
		r = strings.TrimSpace(r)
		delete(set, r)
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
