package resolver

import (
	"sync"

	"github.com/xaionaro-go/binder/tools/pkg/parser"
)

// TypeRegistry maps fully qualified AIDL names to their parsed definitions.
type TypeRegistry struct {
	mu   sync.RWMutex
	Defs map[string]parser.Definition
}

// NewTypeRegistry creates a new empty TypeRegistry.
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		Defs: make(map[string]parser.Definition),
	}
}

// Register adds a definition to the registry under the given qualified name.
func (r *TypeRegistry) Register(
	qualifiedName string,
	def parser.Definition,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Defs[qualifiedName] = def
}

// Lookup returns the definition for the given qualified name, or false if not found.
func (r *TypeRegistry) Lookup(
	qualifiedName string,
) (parser.Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.Defs[qualifiedName]
	return def, ok
}

// All returns a copy of all registered definitions.
func (r *TypeRegistry) All() map[string]parser.Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]parser.Definition, len(r.Defs))
	for k, v := range r.Defs {
		result[k] = v
	}
	return result
}

// LookupByShortName returns the definition whose short name (last segment
// after the final dot) matches the given name. If multiple definitions share
// the same short name, the first match is returned. This is useful for
// resolving unqualified type references within a package.
func (r *TypeRegistry) LookupByShortName(
	shortName string,
) (parser.Definition, bool) {
	_, def, ok := r.lookupByShortName(shortName)
	return def, ok
}

// LookupQualifiedByShortName returns the fully qualified name and definition
// whose short name (last segment after the final dot) matches the given name.
// If multiple definitions share the same short name, the first match is returned.
func (r *TypeRegistry) LookupQualifiedByShortName(
	shortName string,
) (string, parser.Definition, bool) {
	return r.lookupByShortName(shortName)
}

func (r *TypeRegistry) lookupByShortName(
	shortName string,
) (string, parser.Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect all matches to avoid non-deterministic map iteration.
	type candidate struct {
		qualifiedName string
		def           parser.Definition
	}
	var candidates []candidate

	for qualifiedName, def := range r.Defs {
		matched := false
		defShort := def.GetName()
		if defShort == shortName {
			matched = true
		}
		if !matched {
			lastDot := len(qualifiedName) - 1
			for lastDot >= 0 && qualifiedName[lastDot] != '.' {
				lastDot--
			}
			if qualifiedName[lastDot+1:] == shortName {
				matched = true
			}
		}
		if !matched {
			continue
		}
		candidates = append(candidates, candidate{qualifiedName, def})
	}

	if len(candidates) == 0 {
		return "", nil, false
	}

	// Pick the best candidate.  Prefer definitions that have actual content
	// (fields) over empty parcelables, so that a stub like
	// android.hardware.HardwareBuffer (no fields) does not shadow the real
	// android.hardware.graphics.common.HardwareBuffer (with fields).
	best := candidates[0]
	for _, c := range candidates[1:] {
		if betterCandidate(c.qualifiedName, c.def, best.qualifiedName, best.def) {
			best = c
		}
	}

	return best.qualifiedName, best.def, true
}

// betterCandidate returns true if candidate (a) should be preferred over (b)
// during short-name resolution.
func betterCandidate(nameA string, defA parser.Definition, nameB string, defB parser.Definition) bool {
	aFields := parcelableFieldCount(defA)
	bFields := parcelableFieldCount(defB)

	// Prefer the definition with more fields (non-empty over empty stub).
	if aFields != bFields {
		return aFields > bFields
	}

	// Tie-break: alphabetically first for determinism.
	return nameA < nameB
}

// parcelableFieldCount returns the number of fields if the definition is a
// ParcelableDecl, or -1 otherwise.
func parcelableFieldCount(def parser.Definition) int {
	if pd, ok := def.(*parser.ParcelableDecl); ok {
		return len(pd.Fields)
	}
	return -1
}

// LookupAllByShortName returns all fully qualified names and definitions
// whose short name matches the given name.
func (r *TypeRegistry) LookupAllByShortName(
	shortName string,
) []struct {
	QualifiedName string
	Def           parser.Definition
} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []struct {
		QualifiedName string
		Def           parser.Definition
	}

	for qualifiedName, def := range r.Defs {
		matched := false
		defShort := def.GetName()
		if defShort == shortName {
			matched = true
		}
		if !matched {
			lastDot := len(qualifiedName) - 1
			for lastDot >= 0 && qualifiedName[lastDot] != '.' {
				lastDot--
			}
			if qualifiedName[lastDot+1:] == shortName {
				matched = true
			}
		}
		if matched {
			results = append(results, struct {
				QualifiedName string
				Def           parser.Definition
			}{qualifiedName, def})
		}
	}

	return results
}
