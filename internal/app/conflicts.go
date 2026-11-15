package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/ansidev/skill-manager-cli/internal/source"
	"github.com/ansidev/skill-manager-cli/internal/store"
)

// pendingSkill is one apply operation that has been resolved but not yet
// performed.
type pendingSkill struct {
	repository     string
	sourceLocation string
	inRepoPath     string
	skill          source.ResolvedSkill
}

// skillConflict describes one skill name whose requested states or content
// conflict across repositories.
type skillConflict struct {
	Name    string
	Reason  string
	Entries []string
}

type conflictCandidate struct {
	repository string
	state      string
	sourcePath string
	inRepoPath string
}

// detectSkillConflicts groups pending apply operations by effective skill name
// and flags names that appear in more than one repository whose requested
// states differ or whose installed content differs. Identical installed
// content is not a conflict: applying it again is a no-op.
func detectSkillConflicts(pending []pendingSkill) []skillConflict {
	groups := make(map[string][]conflictCandidate)
	order := make([]string, 0, len(pending))
	for _, item := range pending {
		name := item.skill.Entry.Name
		if _, ok := groups[name]; !ok {
			order = append(order, name)
		}
		groups[name] = append(groups[name], conflictCandidate{
			repository: item.repository,
			state:      normalizeSkillState(item.skill.Entry.State),
			sourcePath: item.skill.Path,
			inRepoPath: item.inRepoPath,
		})
	}

	conflicts := make([]skillConflict, 0)
	for _, name := range order {
		candidates := groups[name]
		repositories := make(map[string]bool, len(candidates))
		for _, candidate := range candidates {
			repositories[candidate.repository] = true
		}
		if len(repositories) < 2 {
			continue
		}
		conflict := describeStateConflict(name, candidates)
		if conflict == nil {
			conflict = describeContentConflict(name, candidates)
		}
		if conflict != nil {
			conflicts = append(conflicts, *conflict)
		}
	}
	return conflicts
}

// describeStateConflict reports a conflict when the same skill name is
// requested with different states in different repositories.
func describeStateConflict(name string, candidates []conflictCandidate) *skillConflict {
	states := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		states[candidate.state] = true
	}
	if len(states) < 2 {
		return nil
	}
	descriptions := make([]string, 0, len(candidates))
	sources := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		descriptions = append(descriptions, conflictEntry(candidate))
		sources = append(sources, fmt.Sprintf("%q (%s)", candidate.repository, candidate.state))
	}
	return &skillConflict{
		Name:    name,
		Reason:  fmt.Sprintf("skill %q is requested with conflicting states across repositories: %s", name, strings.Join(sources, ", ")),
		Entries: descriptions,
	}
}

// describeContentConflict reports a conflict when repositories that request the
// same installed skill name provide differing skill directories.
func describeContentConflict(name string, candidates []conflictCandidate) *skillConflict {
	first := &candidates[0]
	for i := 1; i < len(candidates); i++ {
		other := &candidates[i]
		if other.repository == first.repository {
			continue
		}
		equal, err := store.DirectoriesEqual(first.sourcePath, other.sourcePath)
		if err != nil {
			return newContentConflict(name, first, other, fmt.Sprintf("skill %q content could not be compared between repositories %q and %q: %v", name, first.repository, other.repository, err))
		}
		if !equal {
			return newContentConflict(name, first, other, fmt.Sprintf("skill %q content differs between repositories %q and %q", name, first.repository, other.repository))
		}
	}
	return nil
}

func newContentConflict(name string, first, other *conflictCandidate, reason string) *skillConflict {
	return &skillConflict{
		Name:    name,
		Reason:  reason,
		Entries: []string{conflictEntry(*first), conflictEntry(*other)},
	}
}

func conflictEntry(candidate conflictCandidate) string {
	return fmt.Sprintf("Repository: %s (state: %s, path: %s)", candidate.repository, candidate.state, candidate.inRepoPath)
}

// printSkillConflicts reports detected conflicts before any action is
// performed, so the user sees what will not be applied and why.
func printSkillConflicts(writer io.Writer, conflicts []skillConflict) {
	printHeading(writer, "Skill conflicts")
	for _, conflict := range conflicts {
		_, _ = fmt.Fprintf(writer, "  - Skill: %s\n", conflict.Name)
		for _, entry := range conflict.Entries {
			_, _ = fmt.Fprintf(writer, "    - %s\n", entry)
		}
		_, _ = fmt.Fprintf(writer, "    - Reason: %s\n", conflict.Reason)
		_, _ = fmt.Fprintln(writer, "    - The conflicting skill will not be applied; resolve the conflict and re-run apply.")
	}
}
