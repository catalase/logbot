package main

import (
	"strings"
)

type Prefix string

// Nick returnes either nick or server name.
func (raw Prefix) Nick() string {
	prefix := string(raw)

	if strings.HasPrefix(prefix, ":") {
		prefix = prefix[len(":"):]
	}

	if i := strings.Index(prefix, "!"); i >= 0 {
		return prefix[:i]
	}

	if i := strings.Index(prefix, "@"); i >= 0 {
		return prefix[:i]
	}

	return prefix
}

// User returns user in prefix. if user is not in prefix, returns empty string
func (raw Prefix) User() string {
	prefix := string(raw)

	i := strings.Index(prefix, "!")
	if i < 0 {
		return ""
	}

	prefix = prefix[i+1:]
	if i := strings.Index(prefix, "@"); i >= 0 {
		prefix = prefix[:i]
	}

	return prefix
}

func (raw Prefix) Host() string {
	prefix := string(raw)

	i := strings.Index(prefix, "@")
	if i < 0 {
		return ""
	}

	return prefix[i+1:]
}
