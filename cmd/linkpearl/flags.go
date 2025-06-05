// Package main - flags.go provides custom flag types and validation utilities for the linkpearl CLI.
//
// # Overview
//
// This file implements specialized flag handling for the linkpearl command-line interface,
// providing custom flag types that extend the standard flag package functionality.
//
// # Custom Flag Types
//
// stringSliceFlag: A custom flag type that allows multiple values to be specified either by:
//   - Repeating the flag multiple times: --join host1:8080 --join host2:8080
//   - Using comma-separated values: --join host1:8080,host2:8080
//   - Combining both approaches
//
// This is particularly useful for specifying multiple peer addresses when joining a mesh network.
//
// # Validation Functions
//
// validateAddress: Performs basic validation on network addresses to catch obvious errors
// before attempting network operations. The validation ensures addresses follow the expected
// format (host:port or :port) but defers full validation to the network layer.
//
// # Usage Example
//
//	var joinAddrs stringSliceFlag
//	flag.Var(&joinAddrs, "join", "Address to join (can be repeated or comma-separated)")
//
//	// After parsing:
//	// --join host1:8080 --join host2:8080,host3:8080
//	// Results in: []string{"host1:8080", "host2:8080", "host3:8080"}
package main

import (
	"fmt"
	"strings"
)

// stringSliceFlag implements flag.Value for repeated string flags
type stringSliceFlag []string

// String returns the string representation of the flag value
func (s *stringSliceFlag) String() string {
	if s == nil || len(*s) == 0 {
		return ""
	}
	return strings.Join(*s, ",")
}

// Set adds a value to the string slice
func (s *stringSliceFlag) Set(value string) error {
	// Support comma-separated values
	values := strings.Split(value, ",")
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			*s = append(*s, v)
		}
	}
	return nil
}

// Get returns the underlying string slice
func (s *stringSliceFlag) Get() []string {
	if s == nil {
		return nil
	}
	return []string(*s)
}

// validateAddress performs basic validation on network addresses
func validateAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("empty address")
	}

	// Basic validation - just check it's not empty and has a colon
	// Full validation will happen when we try to connect
	if !strings.Contains(addr, ":") && !strings.HasPrefix(addr, ":") {
		return fmt.Errorf("address should be in format host:port or :port")
	}

	return nil
}
