package main

import (
	"fmt"
	"strings"
	"sync"

	"neomega-blocks/ir"
	specific "neomega-blocks/presets/block-converts/specific-converts"
	current "neomega-blocks/presets/java/v26_3"
	"neomega-converter-core/be2je"
)

// Geyser's inverse mapping is not lossless. For newly introduced blocks only,
// choose the candidate closest to its official Java default, then lowest ID.
// Remove waterlogged: the converter reconstructs water from Bedrock layer 1.
var preview26Mapping = sync.OnceValues(func() (be2je.StateFallback, error) {
	catalog, err := current.Open()
	if err != nil {
		return nil, fmt.Errorf("26.3 preview catalog: %w", err)
	}
	best := map[ir.IRBlock]ir.IRBlock{}
	scores := map[ir.IRBlock]int{}
	defaults := map[string]ir.IRBlock{}
	for definition := range catalog.Blocks() {
		state, _ := catalog.ByID(definition.DefaultStateID)
		defaults[definition.Name] = state.Java
	}
	for state := range catalog.States() {
		score := 0
		for key, value := range defaults[state.Java.NameWithNameSpace()].States().GoMap() {
			if state.Java.States().GoMap()[key] == value {
				score++
			}
		}
		if _, found := best[state.Bedrock]; found && scores[state.Bedrock] >= score {
			continue
		}
		best[state.Bedrock] = ir.IRBlock(state.Java.NameWithNameSpace() + " " + string(state.Java.States().Delete("waterlogged"))).Tidy()
		scores[state.Bedrock] = score
	}
	return func(source ir.IRBlock) (ir.IRBlock, bool) {
		// 正向表只保存每个 Java 状态选用的一组 Bedrock 属性，不能覆盖
		// 游戏保存的全部悬挂牌组合。复用原有规则解释朝向，再用新版目录验证。
		if strings.HasPrefix(source.NameWithNameSpace(), "minecraft:") && strings.HasSuffix(source.ShortName(), "_hanging_sign") {
			if converted, ok := specific.BedrockToJavaHangingSign(source); ok {
				if normalized, err := catalog.Normalize(converted); err == nil {
					return ir.IRBlock(normalized.NameWithNameSpace() + " " + string(normalized.States().Delete("waterlogged"))).Tidy(), true
				}
			}
		}
		result, ok := best[source.Tidy()]
		return result, ok
	}, nil
})
