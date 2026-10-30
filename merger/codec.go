package merger

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"

	"neomega-viewer/blockactor"
	"neomega-viewer/effector"
	"neomega-viewer/mesher/geometry"
)

const Version uint32 = 1
const ContentType = "application/vnd.neomega.frame"

type Frame struct {
	// 一份 Frame 表示完整快照，空列表会清空对应通道。
	Mesh    geometry.Mesh
	Effects []effector.Message
	Actors  []blockactor.Actor
}

type header struct {
	Magic    [4]byte
	Version  uint32
	Sections uint32
}

type section struct {
	Type   uint32
	Offset uint32
	Length uint32
}

const (
	meshSection uint32 = iota + 1
	effectSection
	actorSection
)

func (frame Frame) Marshal(w io.Writer) error {
	// NVMS 是完整的子载荷，目录长度不包含段间填充。
	var mesh, actors bytes.Buffer
	if err := frame.Mesh.Marshal(&mesh); err != nil {
		return err
	}
	effects := frame.Effects
	if effects == nil {
		effects = []effector.Message{}
	}
	messages, err := json.Marshal(effects)
	if err != nil {
		return err
	}
	actorWriter := writer{w: &actors}
	actorWriter.write(uint32(len(frame.Actors)))
	for _, actor := range frame.Actors {
		var payload bytes.Buffer
		if err := actor.Mesh.Marshal(&payload); err != nil {
			return err
		}
		actorWriter.write(actor.Pos)
		actorWriter.write(uint8(0))
		actorWriter.write(uint32(payload.Len()))
		actorWriter.write(payload.Bytes())
	}
	if actorWriter.err != nil {
		return actorWriter.err
	}
	payloads := [3][]byte{mesh.Bytes(), messages, actors.Bytes()}
	var directory [3]section
	offset := uint32(binary.Size(header{}) + binary.Size(directory))
	for i, data := range payloads {
		directory[i] = section{Type: uint32(i + 1), Offset: offset, Length: uint32(len(data))}
		offset = align(offset + uint32(len(data)))
	}
	out := writer{w: w}
	out.write(header{Magic: [4]byte{'N', 'V', 'F', 'R'}, Version: Version, Sections: 3})
	out.write(directory)
	for _, data := range payloads {
		out.write(data)
		out.write(make([]byte, align(uint32(len(data)))-uint32(len(data))))
	}
	return out.err
}

func Unmarshal(r io.Reader) (Frame, error) {
	// 用有长度边界的子 reader 解 NVMS，避免把下一段当成网格的尾部。
	var frame Frame
	data, err := io.ReadAll(r)
	if err != nil {
		return frame, err
	}
	input := bytes.NewReader(data)
	var head header
	if err := binary.Read(input, binary.LittleEndian, &head); err != nil {
		return frame, err
	}
	directory := make([]section, head.Sections)
	if err := binary.Read(input, binary.LittleEndian, directory); err != nil {
		return frame, err
	}
	for _, entry := range directory {
		payload := io.NewSectionReader(input, int64(entry.Offset), int64(entry.Length))
		switch entry.Type {
		case meshSection:
			frame.Mesh, err = geometry.UnmarshalMesh(payload)
		case effectSection:
			err = json.NewDecoder(payload).Decode(&frame.Effects)
		case actorSection:
			frame.Actors, err = readActors(payload)
		}
		if err != nil {
			return frame, err
		}
	}
	return frame, nil
}

func readActors(r io.Reader) ([]blockactor.Actor, error) {
	var count uint32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return nil, err
	}
	actors := make([]blockactor.Actor, count)
	for i := range actors {
		var record struct {
			Pos      [3]uint8
			Reserved uint8
			Length   uint32
		}
		if err := binary.Read(r, binary.LittleEndian, &record); err != nil {
			return nil, err
		}
		mesh, err := geometry.UnmarshalMesh(io.LimitReader(r, int64(record.Length)))
		if err != nil {
			return nil, err
		}
		actors[i] = blockactor.Actor{Pos: record.Pos, Mesh: mesh}
	}
	return actors, nil
}

func align(n uint32) uint32 { return (n + 3) &^ 3 }

type writer struct {
	w   io.Writer
	err error
}

func (w *writer) write(value any) {
	// 保留第一个 I/O 错误，字段顺序可直接对应协议布局。
	if w.err == nil {
		w.err = binary.Write(w.w, binary.LittleEndian, value)
	}
}
