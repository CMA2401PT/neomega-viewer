package model

import (
	"slices"
	"strings"

	"neomega-assets/asset"
)

// PropertyValues describes controls useful for a static preview. Conditions
// provide a partial domain, so the current values are included as well.
func PropertyValues(state asset.BlockState, current map[string]string) map[string][]string {
	values := map[string][]string{}
	add := func(key, expression string) {
		if strings.HasPrefix(expression, "!") {
			return
		}
		for _, value := range strings.Split(expression, "|") {
			values[key] = append(values[key], value)
			if value == "true" || value == "false" {
				values[key] = append(values[key], "false", "true")
			}
		}
	}
	for selector := range state.Variants {
		if selector != "" {
			for _, pair := range strings.Split(selector, ",") {
				key, value, _ := strings.Cut(pair, "=")
				add(key, value)
			}
		}
	}
	var visit func(*asset.Condition)
	visit = func(condition *asset.Condition) {
		if condition == nil {
			return
		}
		for key, value := range condition.Properties {
			add(key, value)
		}
		for i := range condition.OR {
			visit(&condition.OR[i])
		}
		for i := range condition.AND {
			visit(&condition.AND[i])
		}
	}
	for _, part := range state.Multipart {
		visit(part.When)
	}
	for key, value := range current {
		add(key, value)
	}
	for key, list := range values {
		slices.Sort(list)
		values[key] = slices.Compact(list)
	}
	return values
}
