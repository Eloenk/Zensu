package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func ParseEpisodeInput(input string, allEps []float64) ([]float64, error) {
	input = strings.TrimSpace(input)
	set := make(map[float64]struct{})

	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if part == "*" {
			for _, ep := range allEps {
				set[ep] = struct{}{}
			}
			continue
		}

		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, err := strconv.ParseFloat(strings.TrimSpace(bounds[0]), 64)
			if err != nil {
				return nil, fmt.Errorf("invalid range start: %q", bounds[0])
			}
			end, err := strconv.ParseFloat(strings.TrimSpace(bounds[1]), 64)
			if err != nil {
				return nil, fmt.Errorf("invalid range end: %q", bounds[1])
			}
			for _, ep := range allEps {
				if ep >= start && ep <= end {
					set[ep] = struct{}{}
				}
			}
			continue
		}

		n, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid episode number: %q", part)
		}
		set[n] = struct{}{}
	}

	result := make([]float64, 0, len(set))
	for ep := range set {
		result = append(result, ep)
	}
	sort.Float64s(result)
	return result, nil
}
