package app

import (
	"sort"
	"strings"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

func sortedEpicKeys(epics map[string]config.EpicConfig) []string {
	keys := make([]string, 0, len(epics))
	for key := range epics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func joinList(values []string, separator string) string {
	return strings.Join(values, separator)
}

func aliasHandles(cfg *config.Config) []string {
	set := map[string]struct{}{}
	for key, epic := range cfg.Epics {
		handle := strings.ToLower(epicAliasHandle(key, epic))
		if handle != "" {
			set[handle] = struct{}{}
		}
		if id := strings.ToLower(strings.TrimSpace(epic.ID)); id != "" {
			set[id] = struct{}{}
		}
	}
	handles := make([]string, 0, len(set))
	for handle := range set {
		handles = append(handles, handle)
	}
	sort.Strings(handles)
	return handles
}

func epicAliasHandle(key string, epic config.EpicConfig) string {
	if alias := strings.ToLower(strings.TrimSpace(epic.Alias)); alias != "" {
		return alias
	}
	return strings.ToLower(key)
}
