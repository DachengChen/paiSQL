// variables.go implements \set variable substitution, similar to psql.
//
// Variables are stored in a simple map and expanded in SQL strings
// before execution. Syntax: :varname is replaced with the value.
package db

import (
	"fmt"
	"strings"
	"sync"
)

// Variables holds user-defined variables for substitution.
type Variables struct {
	mu   sync.RWMutex
	vars map[string]string
}

// NewVariables creates an empty variable store.
func NewVariables() *Variables {
	return &Variables{vars: make(map[string]string)}
}

// Set stores a variable. Usage: \set name value
func (v *Variables) Set(name, value string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.vars[name] = value
}

// Get retrieves a variable value.
func (v *Variables) Get(name string) (string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	val, ok := v.vars[name]
	return val, ok
}

// Unset removes a variable.
func (v *Variables) Unset(name string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.vars, name)
}

// List returns all variables as formatted strings.
func (v *Variables) List() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var result []string
	for k, val := range v.vars {
		result = append(result, fmt.Sprintf("%s = '%s'", k, val))
	}
	return result
}

// Expand replaces :varname occurrences in sql with stored values.
func (v *Variables) Expand(sql string) string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	for name, val := range v.vars {
		sql = strings.ReplaceAll(sql, ":"+name, val)
	}
	return sql
}
