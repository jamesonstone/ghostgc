package worktree

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const maxRegistrations = 256

// ParseRegistrations parses the documented NUL-delimited porcelain format.
func ParseRegistrations(raw []byte) ([]Registration, error) {
	parts := bytes.Split(raw, []byte{0})
	var out []Registration
	var current *Registration
	for _, part := range parts {
		if len(part) == 0 {
			if current != nil {
				out = append(out, *current)
				current = nil
				if len(out) > maxRegistrations {
					return nil, fmt.Errorf("worktree: registration count exceeds %d", maxRegistrations)
				}
			}
			continue
		}
		key, value, _ := bytes.Cut(part, []byte(" "))
		switch string(key) {
		case "worktree":
			if current != nil {
				return nil, fmt.Errorf("worktree: malformed registration boundary")
			}
			current = &Registration{Path: string(value)}
		case "HEAD":
			if current != nil {
				current.HEAD = string(value)
			}
		case "branch":
			if current != nil {
				current.Ref = string(value)
				current.Branch = strings.TrimPrefix(current.Ref, "refs/heads/")
			}
		case "detached":
			if current != nil {
				current.Detached = true
			}
		case "bare":
			if current != nil {
				current.Bare = true
			}
		case "locked":
			if current != nil {
				current.Locked = true
			}
		case "prunable":
			if current != nil {
				current.Prunable = true
			}
		}
	}
	if current != nil {
		out = append(out, *current)
	}
	for _, rec := range out {
		if rec.Path == "" {
			return nil, fmt.Errorf("worktree: registration has no path")
		}
	}
	return out, nil
}

type statusPathCheck func(kind byte, path string) bool

// ParseStatus reduces porcelain v2 into counts and a one-way fingerprint.
func ParseStatus(raw []byte, approved statusPathCheck) (StatusEvidence, error) {
	result := StatusEvidence{}
	fields := bytes.Split(raw, []byte{0})
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) == 0 {
			continue
		}
		switch entry[0] {
		case '1', '2':
			parts := bytes.SplitN(entry, []byte(" "), 9)
			if len(parts) < 9 || len(parts[1]) != 2 {
				return result, fmt.Errorf("worktree: malformed status entry")
			}
			xy := string(parts[1])
			if xy[0] != '.' {
				result.Staged++
			}
			if xy[1] != '.' {
				result.Tracked++
			}
			if entry[0] == '2' {
				if i+1 >= len(fields) || len(fields[i+1]) == 0 {
					return result, fmt.Errorf("worktree: malformed rename status entry")
				}
				i++
			}
		case 'u':
			result.Conflicted++
		case '?', '!':
			path := ""
			if len(entry) > 2 {
				path = string(entry[2:])
			}
			if approved != nil && approved(entry[0], path) {
				continue
			}
			if entry[0] == '?' {
				result.Untracked++
			} else {
				result.Ignored++
			}
		default:
			return result, fmt.Errorf("worktree: unknown status record %q", entry[0])
		}
	}
	sum := sha256.Sum256(raw)
	result.Fingerprint = hex.EncodeToString(sum[:])
	return result, nil
}
