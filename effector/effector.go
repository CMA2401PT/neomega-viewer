package effector

import "neomega-blocks/ir"

type Input struct {
	Pos   [3]uint8
	State ir.IRBlock
}

type Message struct {
	Pos    [3]uint8 `json:"pos"`
	Effect string   `json:"effect"`
}

type Effector struct {
	effects map[string]string
}

func New(effects map[string]string) *Effector {
	// 方块名 → 特效名由组装处给出，启用哪些特效不写死在包里。
	return &Effector{effects: effects}
}

func (e *Effector) BlockNames() []string {
	names := make([]string, 0, len(e.effects))
	for name := range e.effects {
		names = append(names, name)
	}
	return names
}

func (e *Effector) Build(inputs []Input) []Message {
	// 消息只描述摆放位置，动画由消费者按特效名驱动。
	messages := make([]Message, 0, len(inputs))
	for _, input := range inputs {
		messages = append(messages, Message{
			Pos:    input.Pos,
			Effect: e.effects[input.State.NameWithNameSpace()],
		})
	}
	return messages
}
