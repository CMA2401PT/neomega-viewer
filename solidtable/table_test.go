package solidtable

import (
	"bytes"
	"maps"
	"testing"
)

func TestWireFormat(t *testing.T) {
	// 独立样本固定小端布局和每条五字节，避免只验证编解码彼此一致。
	wire := []byte{
		'N', 'V', 'S', 'T', 1, 0, 0, 0, 2, 0, 0, 0,
		4, 3, 2, 1, 0x3f,
		8, 7, 6, 5, 0x10,
	}
	want := Table{0x01020304: 0x3f, 0x05060708: 0x10}
	got, err := Decode(bytes.NewReader(wire))
	if err != nil || !maps.Equal(got, want) {
		t.Fatalf("decoded %v, error %v", got, err)
	}
	var encoded bytes.Buffer
	if err := want.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded.Bytes(), wire) {
		t.Fatalf("encoded %x, want %x", encoded.Bytes(), wire)
	}
}
