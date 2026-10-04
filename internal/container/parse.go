//go:build linux

package container

import (
	"fmt"
	"strconv"
	"strings"

	"cardinal/internal/portmap"
)

// ParsePortMapping expands a port spec ("8080:80", "8000-8010:80/udp",
// "8000-8010:8000-8010") into a list of PortMaps.
func ParsePortMapping(s string) ([]PortMap, error) {
	mappings, err := portmap.Parse(s)
	if err != nil {
		return nil, err
	}
	out := make([]PortMap, 0, len(mappings))
	for _, m := range mappings {
		out = append(out, PortMap{
			HostPort:      m.HostPort,
			ContainerPort: m.ContainerPort,
			Protocol:      m.Protocol,
		})
	}
	return out, nil
}

func ParseDiskString(s string) (int64, error) {
	return ParseMemoryString(s)
}

func ParseMemoryString(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	s = strings.ToUpper(s)
	var mult int64 = 1
	switch {
	case strings.HasSuffix(s, "T"):
		mult = 1024 * 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "T")
	case strings.HasSuffix(s, "G"):
		mult = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "K"):
		mult = 1024
		s = strings.TrimSuffix(s, "K")
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value: %s", s)
	}
	return val * mult, nil
}
