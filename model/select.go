// Package model selects variants/multipart and resolves inherited definitions.
// Neither operation knows about a world, HTTP, or a graphics API.
package model

import (
	"fmt"
	"slices"
	"strings"

	"neomega-assets/asset"
)

func valueMatches(actual, expected string) bool {
	negative := strings.HasPrefix(expected, "!")
	expected = strings.TrimPrefix(expected, "!")
	found := slices.Contains(strings.Split(expected, "|"), actual)
	return found != negative
}

func variantMatches(selector string, properties map[string]string) bool {
	if selector == "" {
		return true
	}
	for _, pair := range strings.Split(selector, ",") {
		key, expected, ok := strings.Cut(pair, "=")
		actual, present := properties[key]
		if !ok || !present || !valueMatches(actual, expected) {
			return false
		}
	}
	return true
}

func matches(condition *asset.Condition, properties map[string]string) bool {
	if condition == nil {
		return true
	}
	for key, expected := range condition.Properties {
		actual, present := properties[key]
		if !present || !valueMatches(actual, expected) {
			return false
		}
	}
	if condition.OR != nil {
		found := false
		for i := range condition.OR {
			found = found || matches(&condition.OR[i], properties)
		}
		if !found {
			return false
		}
	}
	for i := range condition.AND {
		if !matches(&condition.AND[i], properties) {
			return false
		}
	}
	return true
}

// Mix produces a reproducible static choice. A subchunk caller can derive seed
// from coordinates; an isolated block can expose it as a preview option.
func Mix(value uint64) uint64 {
	value += 0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

func choose(choice asset.Choice, seed uint64) (asset.Apply, error) {
	total := uint64(0)
	for _, option := range choice.Options {
		weight := 1
		if option.Weight != nil {
			weight = *option.Weight
		}
		if weight <= 0 {
			return asset.Apply{}, fmt.Errorf("nonpositive model weight")
		}
		total += uint64(weight)
	}
	if total == 0 {
		return asset.Apply{}, fmt.Errorf("empty model choice")
	}
	value := Mix(seed) % total
	for _, option := range choice.Options {
		weight := uint64(1)
		if option.Weight != nil {
			weight = uint64(*option.Weight)
		}
		if value < weight {
			return option, nil
		}
		value -= weight
	}
	panic("weighted choice exhausted")
}

func Select(state asset.BlockState, properties map[string]string, seed uint64) ([]asset.Apply, error) {
	selected := []asset.Apply{}
	keys := make([]string, 0, len(state.Variants))
	for key := range state.Variants {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if variantMatches(key, properties) {
			if len(selected) != 0 {
				return nil, fmt.Errorf("overlapping variant selectors")
			}
			option, err := choose(state.Variants[key], seed)
			if err != nil {
				return nil, err
			}
			selected = append(selected, option)
		}
	}
	if len(state.Variants) != 0 && len(selected) == 0 {
		return nil, fmt.Errorf("no variant matches properties %v", properties)
	}
	for i, part := range state.Multipart {
		if matches(part.When, properties) {
			option, err := choose(part.Apply, seed+uint64(i)+1)
			if err != nil {
				return nil, err
			}
			selected = append(selected, option)
		}
	}
	return selected, nil
}

func SelectFirst(state asset.BlockState, properties map[string]string) ([]asset.Apply, error) {
	// 候选只取首项，命中的 multipart 部件仍需全部保留。
	choices := []asset.Choice{}
	keys := make([]string, 0, len(state.Variants))
	for key := range state.Variants {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if variantMatches(key, properties) {
			choices = append(choices, state.Variants[key])
		}
	}
	if len(state.Variants) != 0 && len(choices) == 0 {
		return nil, fmt.Errorf("no variant matches properties %v", properties)
	}
	for _, part := range state.Multipart {
		if matches(part.When, properties) {
			choices = append(choices, part.Apply)
		}
	}
	selected := make([]asset.Apply, 0, len(choices))
	for _, choice := range choices {
		if len(choice.Options) == 0 {
			return nil, fmt.Errorf("empty model choice")
		}
		selected = append(selected, choice.Options[0])
	}
	return selected, nil
}
