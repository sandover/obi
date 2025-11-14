package app

import (
	"fmt"
	"strings"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

// sessionPlan captures the resolved metadata for a Codex run.
type sessionMode int

const (
	sessionModeWork sessionMode = iota
	sessionModeSummary
)

type sessionPlan struct {
	EpicKey              string
	EpicName             string
	Alias                string
	EpicID               string
	Tool                 string
	EpicPrompt           string
	BasePrompt           string
	Codex                config.CodexConfig
	ResumeEnabled        bool
	ResumeCompletedBeads []string
	RepoRoot             string
	ConfigDigest         string
	Mode                 sessionMode
	SummaryPrompt        string
	SummaryChunks        []summaryChunk
	SummaryIncluded      int
	SummaryTotal         int
	BeadIDOverride       string
}

func prepareSession(cfg *config.Config, requestedAlias string) (sessionPlan, error) {
	key, target, err := resolveEpic(cfg, requestedAlias)
	if err != nil {
		return sessionPlan{}, err
	}
	return sessionPlan{
		EpicKey:    key,
		EpicName:   target.Name,
		Alias:      aliasFromRequest(requestedAlias, key, target),
		EpicID:     target.ID,
		Tool:       target.Tool,
		EpicPrompt: target.Prompt,
		BasePrompt: cfg.BasePrompt,
		Codex:      cfg.EffectiveCodex(target),
	}, nil
}

func resolveEpic(cfg *config.Config, requested string) (string, config.EpicConfig, error) {
	if strings.TrimSpace(requested) == "" {
		return cfg.Epic("")
	}
	if tgt, ok := cfg.Epics[requested]; ok {
		return requested, tgt, nil
	}

	var matchedKey string
	lowerRequested := strings.ToLower(requested)
	for key, tgt := range cfg.Epics {
		handle := strings.ToLower(tgt.Alias)
		if handle == "" {
			handle = strings.ToLower(key)
		}
		if handle == lowerRequested {
			if matchedKey != "" && matchedKey != key {
				return "", config.EpicConfig{}, fmt.Errorf("alias %q matches multiple epics (%s, %s)", requested, matchedKey, key)
			}
			matchedKey = key
		}
	}
	if matchedKey != "" {
		return matchedKey, cfg.Epics[matchedKey], nil
	}
	return "", config.EpicConfig{}, fmt.Errorf("unknown epic or alias %q", requested)
}

func aliasFromRequest(_ string, key string, epic config.EpicConfig) string {
	return epicAliasHandle(key, epic)
}

func (p sessionPlan) resumeSkipSet() map[string]struct{} {
	if len(p.ResumeCompletedBeads) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(p.ResumeCompletedBeads))
	for _, bead := range p.ResumeCompletedBeads {
		normalized := strings.ToLower(strings.TrimSpace(bead))
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}
	}
	return set
}
