package resolver

import (
	"sync"

	"github.com/xaionaro-go/aidl/tools/pkg/parser"
)

// TypeRegistry maps fully qualified AIDL names to their parsed definitions.
type TypeRegistry struct {
	mu   sync.RWMutex
	defs map[string]parser.Definition
}

// NewTypeRegistry creates a new empty TypeRegistry.
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		defs: make(map[string]parser.Definition),
	}
}

// Register adds a definition to the registry under the given qualified name.
func (r *TypeRegistry) Register(
	qualifiedName string,
	def parser.Definition,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defs[qualifiedName] = def
}

// Lookup returns the definition for the given qualified name, or false if not found.
func (r *TypeRegistry) Lookup(
	qualifiedName string,
) (parser.Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.defs[qualifiedName]
	return def, ok
}

// All returns a copy of all registered definitions.
func (r *TypeRegistry) All() map[string]parser.Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]parser.Definition, len(r.defs))
	for k, v := range r.defs {
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
	var bestName string
	var bestDef parser.Definition

	for qualifiedName, def := range r.defs {
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

		if bestName == "" || qualifiedName < bestName {
			bestName = qualifiedName
			bestDef = def
		}
	}

	if bestName != "" {
		return bestName, bestDef, true
	}
	return "", nil, false
}
